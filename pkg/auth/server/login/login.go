package login

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/util"

	"github.com/openshift/origin/pkg/auth/authenticator"
	"github.com/openshift/origin/pkg/auth/oauth/handlers"
	"github.com/openshift/origin/pkg/auth/server/csrf"
)

const (
	thenParam     = "then"
	csrfParam     = "csrf"
	usernameParam = "username"
	passwordParam = "password"
)

type PasswordAuthenticator interface {
	authenticator.Password
	handlers.AuthenticationSuccessHandler
}

type LoginFormRenderer interface {
	Render(form LoginForm, w http.ResponseWriter, req *http.Request)
}

type LoginForm struct {
	Action string
	Error  string
	Names  LoginFormFields
	Values LoginFormFields
}

type LoginFormFields struct {
	Then     string
	CSRF     string
	Username string
	Password string
}

type Login struct {
	csrf   csrf.CSRF
	auth   PasswordAuthenticator
	render LoginFormRenderer
}

func NewLogin(csrf csrf.CSRF, auth PasswordAuthenticator, render LoginFormRenderer) *Login {
	return &Login{
		csrf:   csrf,
		auth:   auth,
		render: render,
	}
}

// Install registers the login handler into a mux. It is expected that the
// provided prefix will serve all operations. Path MUST NOT end in a slash.
func (l *Login) Install(mux Mux, paths ...string) {
	for _, path := range paths {
		path = strings.TrimRight(path, "/")
		mux.HandleFunc(path, l.ServeHTTP)
	}
}

func (l *Login) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		l.handleLoginForm(w, req)
	case "POST":
		l.handleLogin(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (l *Login) handleLoginForm(w http.ResponseWriter, req *http.Request) {
	uri, err := getBaseURL(req)
	if err != nil {
		glog.Errorf("Unable to generate base URL: %v", err)
		http.Error(w, "Unable to determine URL", http.StatusInternalServerError)
		return
	}

	form := LoginForm{
		Action: uri.String(),
		Names: LoginFormFields{
			Then:     thenParam,
			CSRF:     csrfParam,
			Username: usernameParam,
			Password: passwordParam,
		},
	}
	if then := req.URL.Query().Get("then"); then != "" {
		// TODO: sanitize 'then'
		form.Values.Then = then
	}
	switch req.URL.Query().Get("reason") {
	case "":
		break
	case "user required":
		form.Error = "Login is required. Please try again."
	case "token expired":
		form.Error = "Could not check CSRF token. Please try again."
	case "access denied":
		form.Error = "Invalid login or password. Please try again."
	default:
		form.Error = "An unknown error has occurred. Please try again."
	}

	csrf, err := l.csrf.Generate(w, req)
	if err != nil {
		util.HandleError(fmt.Errorf("unable to generate CSRF token: %v", err))
	}
	form.Values.CSRF = csrf

	l.render.Render(form, w, req)
}

func (l *Login) handleLogin(w http.ResponseWriter, req *http.Request) {
	if ok, err := l.csrf.Check(req, req.FormValue("csrf")); !ok || err != nil {
		glog.Errorf("Unable to check CSRF token: %v", err)
		failed("token expired", w, req)
		return
	}
	then := req.FormValue("then")
	user, password := req.FormValue("username"), req.FormValue("password")
	if user == "" {
		failed("user required", w, req)
		return
	}
	context, ok, err := l.auth.AuthenticatePassword(user, password)
	if err != nil {
		glog.Errorf("Unable to authenticate password: %v", err)
		failed("unknown error", w, req)
		return
	}
	if !ok {
		failed("access denied", w, req)
		return
	}
	l.auth.AuthenticationSucceeded(context, then, w, req)
}

// NewLoginFormRenderer creates a login form renderer that takes in an optional custom template to
// allow branding of the login page. Uses the default if customLoginTemplateFile is not set.
func NewLoginFormRenderer(customLoginTemplateFile string) (*loginTemplateRenderer, error) {
	r := &loginTemplateRenderer{}
	if len(customLoginTemplateFile) > 0 {
		customTemplate, err := template.ParseFiles(customLoginTemplateFile)
		if err != nil {
			return nil, err
		}
		r.loginTemplate = customTemplate
	} else {
		r.loginTemplate = defaultLoginTemplate
	}

	return r, nil
}

func ValidateLoginTemplate(templateContent []byte) []error {
	var allErrs []error

	template, err := template.New("loginTemplateTest").Parse(string(templateContent))
	if err != nil {
		return append(allErrs, err)
	}

	// Execute the template with dummy values and check if they're there.
	form := LoginForm{
		Action: "MyAction",
		Error:  "MyError",
		Names: LoginFormFields{
			Then:     "MyThenName",
			CSRF:     "MyCSRFName",
			Username: "MyUsernameName",
			Password: "MyPasswordName",
		},
		Values: LoginFormFields{
			Then:     "MyThenValue",
			CSRF:     "MyCSRFValue",
			Username: "MyUsernameValue",
		},
	}

	var buffer bytes.Buffer
	err = template.Execute(&buffer, form)
	if err != nil {
		return append(allErrs, err)
	}
	output := buffer.Bytes()

	var testFields = map[string]string{
		"Action":          form.Action,
		"Error":           form.Error,
		"Names.Then":      form.Names.Then,
		"Names.CSRF":      form.Values.CSRF,
		"Names.Username":  form.Names.Username,
		"Names.Password":  form.Names.Password,
		"Values.Then":     form.Values.Then,
		"Values.CSRF":     form.Values.CSRF,
		"Values.Username": form.Values.Username,
	}

	for field, value := range testFields {
		if !bytes.Contains(output, []byte(value)) {
			allErrs = append(allErrs, errors.New(fmt.Sprintf("template is missing parameter {{ .%s }}", field)))
		}
	}

	return allErrs
}

type loginTemplateRenderer struct {
	loginTemplate *template.Template
}

func (r loginTemplateRenderer) Render(form LoginForm, w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if err := r.loginTemplate.Execute(w, form); err != nil {
		util.HandleError(fmt.Errorf("unable to render login template: %v", err))
	}
}

// LoginTemplateExample is a basic template for customizing the login page.
const LoginTemplateExample = `<!DOCTYPE html>
<!--

This template can be modified and used to customize the login page. To replace
the login page, set master configuration option oauthConfig.templates.login to
the path of the template file. Don't remove parameters in curly braces below.

oauthConfig:
  templates:
    login: templates/login-template.html

-->
<html>
  <head>
    <title>Login</title>
    <style type="text/css">
      body {
        font-family: "Open Sans", Helvetica, Arial, sans-serif;
        font-size: 14px;
        margin: 15px;
      }

      input {
        margin-bottom: 10px;
        width: 300px;
      }

      .error {
        color: red;
        margin-bottom: 10px;
      }
    </style>
  </head>
  <body>

    {{ if .Error }}
      <div class="error">{{ .Error }}</div>
    {{ end }}

    <form action="{{ .Action }}" method="POST">
      <input type="hidden" name="{{ .Names.Then }}" value="{{ .Values.Then }}">
      <input type="hidden" name="{{ .Names.CSRF }}" value="{{ .Values.CSRF }}">

      <div>
        <label for="inputUsername">Username</label>
      </div>
      <div>
        <input type="text" id="inputUsername" autofocus="autofocus" type="text" name="{{ .Names.Username }}" value="{{ .Values.Username }}">
      </div>

      <div>
        <label for="inputPassword">Password</label>
      </div>
      <div>
        <input type="password" id="inputPassword" type="password" name="{{ .Names.Password }}" value="">
      </div>

      <button type="submit">Log In</button>

    </form>

  </body>
</html>
`

var defaultLoginTemplate = template.Must(template.New("defaultLoginForm").Parse(defaultLoginTemplateString))

const defaultLoginTemplateString = `<!DOCTYPE html>
<!--[if IE 8]><html class="ie8 login-pf"><![endif]-->
<!--[if IE 9]><html class="ie9 login-pf"><![endif]-->
<!--[if gt IE 9]><!-->
<html class="login-pf">
<!--<![endif]-->
  <head>
    <title>Login - Red Hat&reg; OpenShift Enterprise</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <link rel="shortcut icon" href="data:image/ico;base64,AAABAAIAEBAAAAEAIAAoBQAAJgAAACAgAAABACAAKBQAAE4FAAAoAAAAEAAAACAAAAABACAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP///xP///+E////2v////D////w////7v///8H///9RAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP///1H////5//////////////////////v7+/+wsLL/5ubn/////8L///8TAAAAAAAAAAAAAAAAAAAAAP///2X/////////////////////////////////////Tk5Q9j4+Qfv/////////4f7+/hkAAAAAAAAAAP///y/////////////////////////////////Gxsn/e3yA//////0aGh35HR8g+5ycnv92dnm3AAAAAP///wv///+p/////+bm6P6mp6n68fHy/v/////////9x8TB/H14bP3////9hIJ+9wAAAP4AAAD/AAAA/wAAAEw4ODtWQUFE+oqKjf9ISUz6SUtP+P////+zq576BgAA+iMgGvtHS1f5NTpL+jAzP/oBAAD+AgAA/wAAAP8EBQWrAAAAmgAAAP8AAAD/AAAA++Lc0PltaF37ChAm/BAPWf8fHZT/JB+z/yQfuP8mIMH/Jy6+/x4mSP8CAAD/AAAA5QEBAboHBwv/AgAA/wUAAP8rMEj6Hh6L/SUg3f8jI+3/JSLs/ygg2/8pIdj/KCHb/yUi5/8jI+3/ICdK/wEAAO8BAgK5AgAA/wIAAP8jLXb/JSHj/yMj7f8lIeX/Ji98/yAnRv8lJcX/KCHa/yciyP8mINv/IyPt/yItdP8BAADvAAAAlAIAAP8mL4z/IyPt/yUi6v8lLV//AgAA/wcAAP8iJz//JSLr/ygh2v8nKbn/Jy2j/ycup/8KDRL/AAAA4gAAAE0AAAD1Jy+V/yMj7f8mMHT/BQAA/yUvbv8nLa//JSHl/ycgzv8oIdL/JSHk/xcaJ/8FAAD/AgAA/wEBBqMAAAAEAAAAoQIAAP8UFyH/DxAV/yYtsP8oLMT/JyXM/ycgzv8nINL/JiHi/yUly/8CAAD/AgAA/wIFBv8AAABDAAAAAAAAACIAAADxBQAA/wUFAv8nMJ//ICQ8/yIrZP8lIdX/JyPO/yMj7f8lL2//AgAA/wICBf8AAACoAAAAAAAAAAAAAAAAAAAASgICBf8CAAD/Iixn/yMj7f8lI9L/JiDf/yYuif8iK23/Fxgj/wIAAP8AAAHSAAAACgAAAAAAAAAAAAAAAAAAAAAAAAA4AAAA6AAAAP8ZHS//CgoQ/woKD/8bIDX/CgoL/wIAAP8BAQapAAAABwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAkAAABkAQAAvgAAAO4AAAD1AQAA4QAAAKEAAAA7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAoAAAAIAAAAEAAAAABACAAAAAAAAAUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD///8v////nP///9H////j////7/////T////w////5v///9b///+s////RgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD///9c/v7+z///////////////////////////////////////////////////////////////4f7+/nX///8CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD///8P/v7+u////////////////////////////////////////////////////////////////////////////////////9f///8tAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/v7+Ov////////////////////////////////////////////////////////////////////+YmJz0pKSn9//////////////////////+/v5pAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP///1T//////////////////////////////////////////////////////////////////////////7e3uPYAAAD0bm5y+/////7///////////////////+AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD+/v46/////f///////////////////////////////////////////////////////////////////////////////05PUPgAAAD3fn6B+u7u7/v///////////////////9sAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA////Bf///+H/////////////////////////////////////////////////////////////////////////////////////////+zo6O/gAAAL/Kywv+f////n////8//////////7q6uolAAAAAAAAAAAAAAAAAAAAAAAAAAD///+W/////////////////////////////////////////////////////////////////////6ysr/8eHiD/NTY3////////////t7e5+AAAAPoAAAD/CgoK+zY4OvhHR0nvREVH/x0dH9AAAAAAAAAAAAAAAAAAAAAA////N///////////////////////////////////////////////////////////////////////////gICD/0dHSf9wcHT/1dXX///////////3HBwc9QAAAP8AAAD/AAAA/wAAAP8AAAD/AAAA/wAAAF4AAAAAAAAAAAAAAAD///+M//////////////////////////6lpaf3pqap+P/////////////////////////+/////f////z////9//////////z//////////v////pNTU/5AAAA/wAAAP8AAAD/AAAA/wAAAP8AAAD/AAAAvQAAAAAAAAAAf39/BqOjpt+zsrX/7Ozt+///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AAAAAAAAAAAAAAAAAAQAAABVAAAAowAAANMAAADtAAAA9gAAAPEAAADbAAAArwAAAGkAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=">
      <style>
        /* Standalone login -- OpenShift Online/Enterprise edition */
@font-face {
  font-family: 'Open Sans';
  src: url(data:application/x-font-woff;charset=utf-8;base64,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) format('woff');
  font-weight: 400;
  font-style: normal;
}
@font-face {
  font-family: 'Open Sans';
  src: url(data:application/x-font-woff;charset=utf-8;base64,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) format('woff');
  font-weight: 700;
  font-style: normal;
}
.alert {
  padding: 7px;
  margin-bottom: 20px;
  border: 1px solid transparent;
  border-radius: 1px;
}
.alert h4 {
  margin-top: 0;
  color: inherit;
}
.alert .alert-link {
  font-weight: 500;
}
.alert > p,
.alert > ul {
  margin-bottom: 0;
}
.alert > p + p {
  margin-top: 5px;
}
.alert-dismissable {
  padding-right: 27px;
}
.alert-dismissable .close {
  position: relative;
  top: -2px;
  right: -21px;
  color: inherit;
}
.alert-success {
  background-color: #ffffff;
  border-color: #5cb75c;
  color: #333333;
}
.alert-success hr {
  border-top-color: #4cad4c;
}
.alert-success .alert-link {
  color: #1a1a1a;
}
.alert-info {
  background-color: #ffffff;
  border-color: #cccccc;
  color: #333333;
}
.alert-info hr {
  border-top-color: #bfbfbf;
}
.alert-info .alert-link {
  color: #1a1a1a;
}
.alert-warning {
  background-color: #ffffff;
  border-color: #eb7720;
  color: #333333;
}
.alert-warning hr {
  border-top-color: #de6a14;
}
.alert-warning .alert-link {
  color: #1a1a1a;
}
.alert-danger {
  background-color: #ffffff;
  border-color: #c90813;
  color: #333333;
}
.alert-danger hr {
  border-top-color: #b00711;
}
.alert-danger .alert-link {
  color: #1a1a1a;
}
.btn {
  display: inline-block;
  margin-bottom: 0;
  font-weight: 600;
  text-align: center;
  vertical-align: middle;
  cursor: pointer;
  background-image: none;
  border: 1px solid transparent;
  white-space: nowrap;
  padding: 2px 6px;
  font-size: 12px;
  line-height: 1.66666667;
  border-radius: 1px;
  -webkit-user-select: none;
  -moz-user-select: none;
  -ms-user-select: none;
  user-select: none;
}
.btn:focus,
.btn:active:focus,
.btn.active:focus {
  outline: thin dotted;
  outline: 5px auto -webkit-focus-ring-color;
  outline-offset: -2px;
}
.btn:hover,
.btn:focus {
  color: #4d5258;
  text-decoration: none;
}
.btn:active,
.btn.active {
  outline: 0;
  background-image: none;
  -webkit-box-shadow: inset 0 3px 5px rgba(0, 0, 0, 0.125);
  box-shadow: inset 0 3px 5px rgba(0, 0, 0, 0.125);
}
.btn.disabled,
.btn[disabled],
fieldset[disabled] .btn {
  cursor: not-allowed;
  pointer-events: none;
  opacity: 0.65;
  filter: alpha(opacity=65);
  -webkit-box-shadow: none;
  box-shadow: none;
}
.btn-default {
  color: #4d5258;
  background-color: #eeeeee;
  border-color: #b7b7b7;
}
.btn-default:hover,
.btn-default:focus,
.btn-default:active,
.btn-default.active,
.open .dropdown-toggle.btn-default {
  color: #4d5258;
  background-color: #dadada;
  border-color: #989898;
}
.btn-default:active,
.btn-default.active,
.open .dropdown-toggle.btn-default {
  background-image: none;
}
.btn-default.disabled,
.btn-default[disabled],
fieldset[disabled] .btn-default,
.btn-default.disabled:hover,
.btn-default[disabled]:hover,
fieldset[disabled] .btn-default:hover,
.btn-default.disabled:focus,
.btn-default[disabled]:focus,
fieldset[disabled] .btn-default:focus,
.btn-default.disabled:active,
.btn-default[disabled]:active,
fieldset[disabled] .btn-default:active,
.btn-default.disabled.active,
.btn-default[disabled].active,
fieldset[disabled] .btn-default.active {
  background-color: #eeeeee;
  border-color: #b7b7b7;
}
.btn-default .badge {
  color: #eeeeee;
  background-color: #4d5258;
}
.btn-primary {
  color: #ffffff;
  background-color: #189ad1;
  border-color: #267da1;
}
.btn-primary:hover,
.btn-primary:focus,
.btn-primary:active,
.btn-primary.active,
.open .dropdown-toggle.btn-primary {
  color: #ffffff;
  background-color: #147fac;
  border-color: #1a576f;
}
.btn-primary:active,
.btn-primary.active,
.open .dropdown-toggle.btn-primary {
  background-image: none;
}
.btn-primary.disabled,
.btn-primary[disabled],
fieldset[disabled] .btn-primary,
.btn-primary.disabled:hover,
.btn-primary[disabled]:hover,
fieldset[disabled] .btn-primary:hover,
.btn-primary.disabled:focus,
.btn-primary[disabled]:focus,
fieldset[disabled] .btn-primary:focus,
.btn-primary.disabled:active,
.btn-primary[disabled]:active,
fieldset[disabled] .btn-primary:active,
.btn-primary.disabled.active,
.btn-primary[disabled].active,
fieldset[disabled] .btn-primary.active {
  background-color: #189ad1;
  border-color: #267da1;
}
.btn-primary .badge {
  color: #189ad1;
  background-color: #ffffff;
}
.btn-success {
  color: #ffffff;
  background-color: #5cb75c;
  border-color: #4cad4c;
}
.btn-success:hover,
.btn-success:focus,
.btn-success:active,
.btn-success.active,
.open .dropdown-toggle.btn-success {
  color: #ffffff;
  background-color: #48a248;
  border-color: #3a833a;
}
.btn-success:active,
.btn-success.active,
.open .dropdown-toggle.btn-success {
  background-image: none;
}
.btn-success.disabled,
.btn-success[disabled],
fieldset[disabled] .btn-success,
.btn-success.disabled:hover,
.btn-success[disabled]:hover,
fieldset[disabled] .btn-success:hover,
.btn-success.disabled:focus,
.btn-success[disabled]:focus,
fieldset[disabled] .btn-success:focus,
.btn-success.disabled:active,
.btn-success[disabled]:active,
fieldset[disabled] .btn-success:active,
.btn-success.disabled.active,
.btn-success[disabled].active,
fieldset[disabled] .btn-success.active {
  background-color: #5cb75c;
  border-color: #4cad4c;
}
.btn-success .badge {
  color: #5cb75c;
  background-color: #ffffff;
}
.btn-info {
  color: #ffffff;
  background-color: #27799c;
  border-color: #226988;
}
.btn-info:hover,
.btn-info:focus,
.btn-info:active,
.btn-info.active,
.open .dropdown-toggle.btn-info {
  color: #ffffff;
  background-color: #1f607b;
  border-color: #164357;
}
.btn-info:active,
.btn-info.active,
.open .dropdown-toggle.btn-info {
  background-image: none;
}
.btn-info.disabled,
.btn-info[disabled],
fieldset[disabled] .btn-info,
.btn-info.disabled:hover,
.btn-info[disabled]:hover,
fieldset[disabled] .btn-info:hover,
.btn-info.disabled:focus,
.btn-info[disabled]:focus,
fieldset[disabled] .btn-info:focus,
.btn-info.disabled:active,
.btn-info[disabled]:active,
fieldset[disabled] .btn-info:active,
.btn-info.disabled.active,
.btn-info[disabled].active,
fieldset[disabled] .btn-info.active {
  background-color: #27799c;
  border-color: #226988;
}
.btn-info .badge {
  color: #27799c;
  background-color: #ffffff;
}
.btn-warning {
  color: #ffffff;
  background-color: #eb7720;
  border-color: #de6a14;
}
.btn-warning:hover,
.btn-warning:focus,
.btn-warning:active,
.btn-warning.active,
.open .dropdown-toggle.btn-warning {
  color: #ffffff;
  background-color: #d06413;
  border-color: #a54f0f;
}
.btn-warning:active,
.btn-warning.active,
.open .dropdown-toggle.btn-warning {
  background-image: none;
}
.btn-warning.disabled,
.btn-warning[disabled],
fieldset[disabled] .btn-warning,
.btn-warning.disabled:hover,
.btn-warning[disabled]:hover,
fieldset[disabled] .btn-warning:hover,
.btn-warning.disabled:focus,
.btn-warning[disabled]:focus,
fieldset[disabled] .btn-warning:focus,
.btn-warning.disabled:active,
.btn-warning[disabled]:active,
fieldset[disabled] .btn-warning:active,
.btn-warning.disabled.active,
.btn-warning[disabled].active,
fieldset[disabled] .btn-warning.active {
  background-color: #eb7720;
  border-color: #de6a14;
}
.btn-warning .badge {
  color: #eb7720;
  background-color: #ffffff;
}
.btn-danger {
  color: #ffffff;
  background-color: #ab070f;
  border-color: #781919;
}
.btn-danger:hover,
.btn-danger:focus,
.btn-danger:active,
.btn-danger.active,
.open .dropdown-toggle.btn-danger {
  color: #ffffff;
  background-color: #84050c;
  border-color: #450e0e;
}
.btn-danger:active,
.btn-danger.active,
.open .dropdown-toggle.btn-danger {
  background-image: none;
}
.btn-danger.disabled,
.btn-danger[disabled],
fieldset[disabled] .btn-danger,
.btn-danger.disabled:hover,
.btn-danger[disabled]:hover,
fieldset[disabled] .btn-danger:hover,
.btn-danger.disabled:focus,
.btn-danger[disabled]:focus,
fieldset[disabled] .btn-danger:focus,
.btn-danger.disabled:active,
.btn-danger[disabled]:active,
fieldset[disabled] .btn-danger:active,
.btn-danger.disabled.active,
.btn-danger[disabled].active,
fieldset[disabled] .btn-danger.active {
  background-color: #ab070f;
  border-color: #781919;
}
.btn-danger .badge {
  color: #ab070f;
  background-color: #ffffff;
}
.btn-link {
  color: #0099d3;
  font-weight: normal;
  cursor: pointer;
  border-radius: 0;
}
.btn-link,
.btn-link:active,
.btn-link[disabled],
fieldset[disabled] .btn-link {
  background-color: transparent;
  -webkit-box-shadow: none;
  box-shadow: none;
}
.btn-link,
.btn-link:hover,
.btn-link:focus,
.btn-link:active {
  border-color: transparent;
}
.btn-link:hover,
.btn-link:focus {
  color: #00618a;
  text-decoration: underline;
  background-color: transparent;
}
.btn-link[disabled]:hover,
fieldset[disabled] .btn-link:hover,
.btn-link[disabled]:focus,
fieldset[disabled] .btn-link:focus {
  color: #999999;
  text-decoration: none;
}
.btn-lg {
  padding: 6px 10px;
  font-size: 14px;
  line-height: 1.33;
  border-radius: 1px;
}
.btn-sm {
  padding: 2px 6px;
  font-size: 11px;
  line-height: 1.5;
  border-radius: 1px;
}
.btn-xs {
  padding: 1px 5px;
  font-size: 11px;
  line-height: 1.5;
  border-radius: 1px;
}
.btn-block {
  display: block;
  width: 100%;
  padding-left: 0;
  padding-right: 0;
}
.btn-block + .btn-block {
  margin-top: 5px;
}
input[type="submit"].btn-block,
input[type="reset"].btn-block,
input[type="button"].btn-block {
  width: 100%;
}
.fade {
  opacity: 0;
  -webkit-transition: opacity 0.15s linear;
  transition: opacity 0.15s linear;
}
.fade.in {
  opacity: 1;
}
.collapse {
  display: none;
}
.collapse.in {
  display: block;
}
.collapsing {
  position: relative;
  height: 0;
  overflow: hidden;
  -webkit-transition: height 0.35s ease;
  transition: height 0.35s ease;
}
fieldset {
  padding: 0;
  margin: 0;
  border: 0;
  min-width: 0;
}
legend {
  display: block;
  width: 100%;
  padding: 0;
  margin-bottom: 20px;
  font-size: 18px;
  line-height: inherit;
  color: #333333;
  border: 0;
  border-bottom: 1px solid #e5e5e5;
}
label {
  display: inline-block;
  margin-bottom: 5px;
  font-weight: bold;
}
input[type="search"] {
  -webkit-box-sizing: border-box;
  -moz-box-sizing: border-box;
  box-sizing: border-box;
}
input[type="radio"],
input[type="checkbox"] {
  margin: 4px 0 0;
  margin-top: 1px \9;
  /* IE8-9 */
  line-height: normal;
}
input[type="file"] {
  display: block;
}
input[type="range"] {
  display: block;
  width: 100%;
}
select[multiple],
select[size] {
  height: auto;
}
input[type="file"]:focus,
input[type="radio"]:focus,
input[type="checkbox"]:focus {
  outline: thin dotted;
  outline: 5px auto -webkit-focus-ring-color;
  outline-offset: -2px;
}
output {
  display: block;
  padding-top: 3px;
  font-size: 12px;
  line-height: 1.66666667;
  color: #333333;
}
.form-control {
  display: block;
  width: 100%;
  height: 26px;
  padding: 2px 6px;
  font-size: 12px;
  line-height: 1.66666667;
  color: #333333;
  background-color: #ffffff;
  background-image: none;
  border: 1px solid #bababa;
  border-radius: 1px;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
  -webkit-transition: border-color ease-in-out .15s, box-shadow ease-in-out .15s;
  transition: border-color ease-in-out .15s, box-shadow ease-in-out .15s;
}
.form-control:focus {
  border-color: #66afe9;
  outline: 0;
  -webkit-box-shadow: inset 0 1px 1px rgba(0,0,0,.075), 0 0 8px rgba(102, 175, 233, 0.6);
  box-shadow: inset 0 1px 1px rgba(0,0,0,.075), 0 0 8px rgba(102, 175, 233, 0.6);
}
.form-control::-moz-placeholder {
  color: #999999;
  opacity: 1;
}
.form-control:-ms-input-placeholder {
  color: #999999;
}
.form-control::-webkit-input-placeholder {
  color: #999999;
}
.form-control:-moz-placeholder {
  color: #999999;
  font-style: italic;
}
.form-control::-moz-placeholder {
  color: #999999;
  font-style: italic;
}
.form-control:-ms-input-placeholder {
  color: #999999;
  font-style: italic;
}
.form-control::-webkit-input-placeholder {
  color: #999999;
  font-style: italic;
}
.form-control[disabled],
.form-control[readonly],
fieldset[disabled] .form-control {
  cursor: not-allowed;
  background-color: #f8f8f8;
  opacity: 1;
}
textarea.form-control {
  height: auto;
}
input[type="search"] {
  -webkit-appearance: none;
}
input[type="date"] {
  line-height: 26px;
}
.form-group {
  margin-bottom: 15px;
}
.radio,
.checkbox {
  display: block;
  min-height: 20px;
  margin-top: 10px;
  margin-bottom: 10px;
  padding-left: 20px;
}
.radio label,
.checkbox label {
  display: inline;
  font-weight: normal;
  cursor: pointer;
}
.radio input[type="radio"],
.radio-inline input[type="radio"],
.checkbox input[type="checkbox"],
.checkbox-inline input[type="checkbox"] {
  float: left;
  margin-left: -20px;
}
.radio + .radio,
.checkbox + .checkbox {
  margin-top: -5px;
}
.radio-inline,
.checkbox-inline {
  display: inline-block;
  padding-left: 20px;
  margin-bottom: 0;
  vertical-align: middle;
  font-weight: normal;
  cursor: pointer;
}
.radio-inline + .radio-inline,
.checkbox-inline + .checkbox-inline {
  margin-top: 0;
  margin-left: 10px;
}
input[type="radio"][disabled],
input[type="checkbox"][disabled],
.radio[disabled],
.radio-inline[disabled],
.checkbox[disabled],
.checkbox-inline[disabled],
fieldset[disabled] input[type="radio"],
fieldset[disabled] input[type="checkbox"],
fieldset[disabled] .radio,
fieldset[disabled] .radio-inline,
fieldset[disabled] .checkbox,
fieldset[disabled] .checkbox-inline {
  cursor: not-allowed;
}
.input-sm {
  height: 22px;
  padding: 2px 6px;
  font-size: 11px;
  line-height: 1.5;
  border-radius: 1px;
}
select.input-sm {
  height: 22px;
  line-height: 22px;
}
textarea.input-sm,
select[multiple].input-sm {
  height: auto;
}
.input-lg {
  height: 33px;
  padding: 6px 10px;
  font-size: 14px;
  line-height: 1.33;
  border-radius: 1px;
}
select.input-lg {
  height: 33px;
  line-height: 33px;
}
textarea.input-lg,
select[multiple].input-lg {
  height: auto;
}
.has-feedback {
  position: relative;
}
.has-feedback .form-control {
  padding-right: 32.5px;
}
.has-feedback .form-control-feedback {
  position: absolute;
  top: 25px;
  right: 0;
  display: block;
  width: 26px;
  height: 26px;
  line-height: 26px;
  text-align: center;
}
.has-success .help-block,
.has-success .control-label,
.has-success .radio,
.has-success .checkbox,
.has-success .radio-inline,
.has-success .checkbox-inline {
  color: #3c763d;
}
.has-success .form-control {
  border-color: #3c763d;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
}
.has-success .form-control:focus {
  border-color: #2b542c;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #67b168;
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #67b168;
}
.has-success .input-group-addon {
  color: #3c763d;
  border-color: #3c763d;
  background-color: #dff0d8;
}
.has-success .form-control-feedback {
  color: #3c763d;
}
.has-warning .help-block,
.has-warning .control-label,
.has-warning .radio,
.has-warning .checkbox,
.has-warning .radio-inline,
.has-warning .checkbox-inline {
  color: #8a6d3b;
}
.has-warning .form-control {
  border-color: #8a6d3b;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
}
.has-warning .form-control:focus {
  border-color: #66512c;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #c0a16b;
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #c0a16b;
}
.has-warning .input-group-addon {
  color: #8a6d3b;
  border-color: #8a6d3b;
  background-color: #fcf8e3;
}
.has-warning .form-control-feedback {
  color: #8a6d3b;
}
.has-error .help-block,
.has-error .control-label,
.has-error .radio,
.has-error .checkbox,
.has-error .radio-inline,
.has-error .checkbox-inline {
  color: #a94442;
}
.has-error .form-control {
  border-color: #a94442;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075);
}
.has-error .form-control:focus {
  border-color: #843534;
  -webkit-box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #ce8483;
  box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.075), 0 0 6px #ce8483;
}
.has-error .input-group-addon {
  color: #a94442;
  border-color: #a94442;
  background-color: #f2dede;
}
.has-error .form-control-feedback {
  color: #a94442;
}
.form-control-static {
  margin-bottom: 0;
}
.help-block {
  display: block;
  margin-top: 5px;
  margin-bottom: 10px;
  color: #737373;
}
@media (min-width: 768px) {
  .form-inline .form-group {
    display: inline-block;
    margin-bottom: 0;
    vertical-align: middle;
  }
  .form-inline .form-control {
    display: inline-block;
    width: auto;
    vertical-align: middle;
  }
  .form-inline .input-group > .form-control {
    width: 100%;
  }
  .form-inline .control-label {
    margin-bottom: 0;
    vertical-align: middle;
  }
  .form-inline .radio,
  .form-inline .checkbox {
    display: inline-block;
    margin-top: 0;
    margin-bottom: 0;
    padding-left: 0;
    vertical-align: middle;
  }
  .form-inline .radio input[type="radio"],
  .form-inline .checkbox input[type="checkbox"] {
    float: none;
    margin-left: 0;
  }
  .form-inline .has-feedback .form-control-feedback {
    top: 0;
  }
}
.form-horizontal .control-label,
.form-horizontal .radio,
.form-horizontal .checkbox,
.form-horizontal .radio-inline,
.form-horizontal .checkbox-inline {
  margin-top: 0;
  margin-bottom: 0;
  padding-top: 3px;
}
.form-horizontal .radio,
.form-horizontal .checkbox {
  min-height: 23px;
}
.form-horizontal .form-group {
  margin-left: -20px;
  margin-right: -20px;
}
.form-horizontal .form-control-static {
  padding-top: 3px;
}
@media (min-width: 768px) {
  .form-horizontal .control-label {
    text-align: right;
  }
}
.form-horizontal .has-feedback .form-control-feedback {
  top: 0;
  right: 20px;
}
.container {
  margin-right: auto;
  margin-left: auto;
  padding-left: 20px;
  padding-right: 20px;
}
@media (min-width: 768px) {
  .container {
    width: 760px;
  }
}
@media (min-width: 992px) {
  .container {
    width: 980px;
  }
}
@media (min-width: 1200px) {
  .container {
    width: 1180px;
  }
}
.container-fluid {
  margin-right: auto;
  margin-left: auto;
  padding-left: 20px;
  padding-right: 20px;
}
.row {
  margin-left: -20px;
  margin-right: -20px;
}
.col-xs-1, .col-sm-1, .col-md-1, .col-lg-1, .col-xs-2, .col-sm-2, .col-md-2, .col-lg-2, .col-xs-3, .col-sm-3, .col-md-3, .col-lg-3, .col-xs-4, .col-sm-4, .col-md-4, .col-lg-4, .col-xs-5, .col-sm-5, .col-md-5, .col-lg-5, .col-xs-6, .col-sm-6, .col-md-6, .col-lg-6, .col-xs-7, .col-sm-7, .col-md-7, .col-lg-7, .col-xs-8, .col-sm-8, .col-md-8, .col-lg-8, .col-xs-9, .col-sm-9, .col-md-9, .col-lg-9, .col-xs-10, .col-sm-10, .col-md-10, .col-lg-10, .col-xs-11, .col-sm-11, .col-md-11, .col-lg-11, .col-xs-12, .col-sm-12, .col-md-12, .col-lg-12 {
  position: relative;
  min-height: 1px;
  padding-left: 20px;
  padding-right: 20px;
}
.col-xs-1, .col-xs-2, .col-xs-3, .col-xs-4, .col-xs-5, .col-xs-6, .col-xs-7, .col-xs-8, .col-xs-9, .col-xs-10, .col-xs-11, .col-xs-12 {
  float: left;
}
.col-xs-12 {
  width: 100%;
}
.col-xs-11 {
  width: 91.66666666666666%;
}
.col-xs-10 {
  width: 83.33333333333334%;
}
.col-xs-9 {
  width: 75%;
}
.col-xs-8 {
  width: 66.66666666666666%;
}
.col-xs-7 {
  width: 58.333333333333336%;
}
.col-xs-6 {
  width: 50%;
}
.col-xs-5 {
  width: 41.66666666666667%;
}
.col-xs-4 {
  width: 33.33333333333333%;
}
.col-xs-3 {
  width: 25%;
}
.col-xs-2 {
  width: 16.666666666666664%;
}
.col-xs-1 {
  width: 8.333333333333332%;
}
.col-xs-pull-12 {
  right: 100%;
}
.col-xs-pull-11 {
  right: 91.66666666666666%;
}
.col-xs-pull-10 {
  right: 83.33333333333334%;
}
.col-xs-pull-9 {
  right: 75%;
}
.col-xs-pull-8 {
  right: 66.66666666666666%;
}
.col-xs-pull-7 {
  right: 58.333333333333336%;
}
.col-xs-pull-6 {
  right: 50%;
}
.col-xs-pull-5 {
  right: 41.66666666666667%;
}
.col-xs-pull-4 {
  right: 33.33333333333333%;
}
.col-xs-pull-3 {
  right: 25%;
}
.col-xs-pull-2 {
  right: 16.666666666666664%;
}
.col-xs-pull-1 {
  right: 8.333333333333332%;
}
.col-xs-pull-0 {
  right: 0%;
}
.col-xs-push-12 {
  left: 100%;
}
.col-xs-push-11 {
  left: 91.66666666666666%;
}
.col-xs-push-10 {
  left: 83.33333333333334%;
}
.col-xs-push-9 {
  left: 75%;
}
.col-xs-push-8 {
  left: 66.66666666666666%;
}
.col-xs-push-7 {
  left: 58.333333333333336%;
}
.col-xs-push-6 {
  left: 50%;
}
.col-xs-push-5 {
  left: 41.66666666666667%;
}
.col-xs-push-4 {
  left: 33.33333333333333%;
}
.col-xs-push-3 {
  left: 25%;
}
.col-xs-push-2 {
  left: 16.666666666666664%;
}
.col-xs-push-1 {
  left: 8.333333333333332%;
}
.col-xs-push-0 {
  left: 0%;
}
.col-xs-offset-12 {
  margin-left: 100%;
}
.col-xs-offset-11 {
  margin-left: 91.66666666666666%;
}
.col-xs-offset-10 {
  margin-left: 83.33333333333334%;
}
.col-xs-offset-9 {
  margin-left: 75%;
}
.col-xs-offset-8 {
  margin-left: 66.66666666666666%;
}
.col-xs-offset-7 {
  margin-left: 58.333333333333336%;
}
.col-xs-offset-6 {
  margin-left: 50%;
}
.col-xs-offset-5 {
  margin-left: 41.66666666666667%;
}
.col-xs-offset-4 {
  margin-left: 33.33333333333333%;
}
.col-xs-offset-3 {
  margin-left: 25%;
}
.col-xs-offset-2 {
  margin-left: 16.666666666666664%;
}
.col-xs-offset-1 {
  margin-left: 8.333333333333332%;
}
.col-xs-offset-0 {
  margin-left: 0%;
}
@media (min-width: 768px) {
  .col-sm-1, .col-sm-2, .col-sm-3, .col-sm-4, .col-sm-5, .col-sm-6, .col-sm-7, .col-sm-8, .col-sm-9, .col-sm-10, .col-sm-11, .col-sm-12 {
    float: left;
  }
  .col-sm-12 {
    width: 100%;
  }
  .col-sm-11 {
    width: 91.66666666666666%;
  }
  .col-sm-10 {
    width: 83.33333333333334%;
  }
  .col-sm-9 {
    width: 75%;
  }
  .col-sm-8 {
    width: 66.66666666666666%;
  }
  .col-sm-7 {
    width: 58.333333333333336%;
  }
  .col-sm-6 {
    width: 50%;
  }
  .col-sm-5 {
    width: 41.66666666666667%;
  }
  .col-sm-4 {
    width: 33.33333333333333%;
  }
  .col-sm-3 {
    width: 25%;
  }
  .col-sm-2 {
    width: 16.666666666666664%;
  }
  .col-sm-1 {
    width: 8.333333333333332%;
  }
  .col-sm-pull-12 {
    right: 100%;
  }
  .col-sm-pull-11 {
    right: 91.66666666666666%;
  }
  .col-sm-pull-10 {
    right: 83.33333333333334%;
  }
  .col-sm-pull-9 {
    right: 75%;
  }
  .col-sm-pull-8 {
    right: 66.66666666666666%;
  }
  .col-sm-pull-7 {
    right: 58.333333333333336%;
  }
  .col-sm-pull-6 {
    right: 50%;
  }
  .col-sm-pull-5 {
    right: 41.66666666666667%;
  }
  .col-sm-pull-4 {
    right: 33.33333333333333%;
  }
  .col-sm-pull-3 {
    right: 25%;
  }
  .col-sm-pull-2 {
    right: 16.666666666666664%;
  }
  .col-sm-pull-1 {
    right: 8.333333333333332%;
  }
  .col-sm-pull-0 {
    right: 0%;
  }
  .col-sm-push-12 {
    left: 100%;
  }
  .col-sm-push-11 {
    left: 91.66666666666666%;
  }
  .col-sm-push-10 {
    left: 83.33333333333334%;
  }
  .col-sm-push-9 {
    left: 75%;
  }
  .col-sm-push-8 {
    left: 66.66666666666666%;
  }
  .col-sm-push-7 {
    left: 58.333333333333336%;
  }
  .col-sm-push-6 {
    left: 50%;
  }
  .col-sm-push-5 {
    left: 41.66666666666667%;
  }
  .col-sm-push-4 {
    left: 33.33333333333333%;
  }
  .col-sm-push-3 {
    left: 25%;
  }
  .col-sm-push-2 {
    left: 16.666666666666664%;
  }
  .col-sm-push-1 {
    left: 8.333333333333332%;
  }
  .col-sm-push-0 {
    left: 0%;
  }
  .col-sm-offset-12 {
    margin-left: 100%;
  }
  .col-sm-offset-11 {
    margin-left: 91.66666666666666%;
  }
  .col-sm-offset-10 {
    margin-left: 83.33333333333334%;
  }
  .col-sm-offset-9 {
    margin-left: 75%;
  }
  .col-sm-offset-8 {
    margin-left: 66.66666666666666%;
  }
  .col-sm-offset-7 {
    margin-left: 58.333333333333336%;
  }
  .col-sm-offset-6 {
    margin-left: 50%;
  }
  .col-sm-offset-5 {
    margin-left: 41.66666666666667%;
  }
  .col-sm-offset-4 {
    margin-left: 33.33333333333333%;
  }
  .col-sm-offset-3 {
    margin-left: 25%;
  }
  .col-sm-offset-2 {
    margin-left: 16.666666666666664%;
  }
  .col-sm-offset-1 {
    margin-left: 8.333333333333332%;
  }
  .col-sm-offset-0 {
    margin-left: 0%;
  }
}
@media (min-width: 992px) {
  .col-md-1, .col-md-2, .col-md-3, .col-md-4, .col-md-5, .col-md-6, .col-md-7, .col-md-8, .col-md-9, .col-md-10, .col-md-11, .col-md-12 {
    float: left;
  }
  .col-md-12 {
    width: 100%;
  }
  .col-md-11 {
    width: 91.66666666666666%;
  }
  .col-md-10 {
    width: 83.33333333333334%;
  }
  .col-md-9 {
    width: 75%;
  }
  .col-md-8 {
    width: 66.66666666666666%;
  }
  .col-md-7 {
    width: 58.333333333333336%;
  }
  .col-md-6 {
    width: 50%;
  }
  .col-md-5 {
    width: 41.66666666666667%;
  }
  .col-md-4 {
    width: 33.33333333333333%;
  }
  .col-md-3 {
    width: 25%;
  }
  .col-md-2 {
    width: 16.666666666666664%;
  }
  .col-md-1 {
    width: 8.333333333333332%;
  }
  .col-md-pull-12 {
    right: 100%;
  }
  .col-md-pull-11 {
    right: 91.66666666666666%;
  }
  .col-md-pull-10 {
    right: 83.33333333333334%;
  }
  .col-md-pull-9 {
    right: 75%;
  }
  .col-md-pull-8 {
    right: 66.66666666666666%;
  }
  .col-md-pull-7 {
    right: 58.333333333333336%;
  }
  .col-md-pull-6 {
    right: 50%;
  }
  .col-md-pull-5 {
    right: 41.66666666666667%;
  }
  .col-md-pull-4 {
    right: 33.33333333333333%;
  }
  .col-md-pull-3 {
    right: 25%;
  }
  .col-md-pull-2 {
    right: 16.666666666666664%;
  }
  .col-md-pull-1 {
    right: 8.333333333333332%;
  }
  .col-md-pull-0 {
    right: 0%;
  }
  .col-md-push-12 {
    left: 100%;
  }
  .col-md-push-11 {
    left: 91.66666666666666%;
  }
  .col-md-push-10 {
    left: 83.33333333333334%;
  }
  .col-md-push-9 {
    left: 75%;
  }
  .col-md-push-8 {
    left: 66.66666666666666%;
  }
  .col-md-push-7 {
    left: 58.333333333333336%;
  }
  .col-md-push-6 {
    left: 50%;
  }
  .col-md-push-5 {
    left: 41.66666666666667%;
  }
  .col-md-push-4 {
    left: 33.33333333333333%;
  }
  .col-md-push-3 {
    left: 25%;
  }
  .col-md-push-2 {
    left: 16.666666666666664%;
  }
  .col-md-push-1 {
    left: 8.333333333333332%;
  }
  .col-md-push-0 {
    left: 0%;
  }
  .col-md-offset-12 {
    margin-left: 100%;
  }
  .col-md-offset-11 {
    margin-left: 91.66666666666666%;
  }
  .col-md-offset-10 {
    margin-left: 83.33333333333334%;
  }
  .col-md-offset-9 {
    margin-left: 75%;
  }
  .col-md-offset-8 {
    margin-left: 66.66666666666666%;
  }
  .col-md-offset-7 {
    margin-left: 58.333333333333336%;
  }
  .col-md-offset-6 {
    margin-left: 50%;
  }
  .col-md-offset-5 {
    margin-left: 41.66666666666667%;
  }
  .col-md-offset-4 {
    margin-left: 33.33333333333333%;
  }
  .col-md-offset-3 {
    margin-left: 25%;
  }
  .col-md-offset-2 {
    margin-left: 16.666666666666664%;
  }
  .col-md-offset-1 {
    margin-left: 8.333333333333332%;
  }
  .col-md-offset-0 {
    margin-left: 0%;
  }
}
@media (min-width: 1200px) {
  .col-lg-1, .col-lg-2, .col-lg-3, .col-lg-4, .col-lg-5, .col-lg-6, .col-lg-7, .col-lg-8, .col-lg-9, .col-lg-10, .col-lg-11, .col-lg-12 {
    float: left;
  }
  .col-lg-12 {
    width: 100%;
  }
  .col-lg-11 {
    width: 91.66666666666666%;
  }
  .col-lg-10 {
    width: 83.33333333333334%;
  }
  .col-lg-9 {
    width: 75%;
  }
  .col-lg-8 {
    width: 66.66666666666666%;
  }
  .col-lg-7 {
    width: 58.333333333333336%;
  }
  .col-lg-6 {
    width: 50%;
  }
  .col-lg-5 {
    width: 41.66666666666667%;
  }
  .col-lg-4 {
    width: 33.33333333333333%;
  }
  .col-lg-3 {
    width: 25%;
  }
  .col-lg-2 {
    width: 16.666666666666664%;
  }
  .col-lg-1 {
    width: 8.333333333333332%;
  }
  .col-lg-pull-12 {
    right: 100%;
  }
  .col-lg-pull-11 {
    right: 91.66666666666666%;
  }
  .col-lg-pull-10 {
    right: 83.33333333333334%;
  }
  .col-lg-pull-9 {
    right: 75%;
  }
  .col-lg-pull-8 {
    right: 66.66666666666666%;
  }
  .col-lg-pull-7 {
    right: 58.333333333333336%;
  }
  .col-lg-pull-6 {
    right: 50%;
  }
  .col-lg-pull-5 {
    right: 41.66666666666667%;
  }
  .col-lg-pull-4 {
    right: 33.33333333333333%;
  }
  .col-lg-pull-3 {
    right: 25%;
  }
  .col-lg-pull-2 {
    right: 16.666666666666664%;
  }
  .col-lg-pull-1 {
    right: 8.333333333333332%;
  }
  .col-lg-pull-0 {
    right: 0%;
  }
  .col-lg-push-12 {
    left: 100%;
  }
  .col-lg-push-11 {
    left: 91.66666666666666%;
  }
  .col-lg-push-10 {
    left: 83.33333333333334%;
  }
  .col-lg-push-9 {
    left: 75%;
  }
  .col-lg-push-8 {
    left: 66.66666666666666%;
  }
  .col-lg-push-7 {
    left: 58.333333333333336%;
  }
  .col-lg-push-6 {
    left: 50%;
  }
  .col-lg-push-5 {
    left: 41.66666666666667%;
  }
  .col-lg-push-4 {
    left: 33.33333333333333%;
  }
  .col-lg-push-3 {
    left: 25%;
  }
  .col-lg-push-2 {
    left: 16.666666666666664%;
  }
  .col-lg-push-1 {
    left: 8.333333333333332%;
  }
  .col-lg-push-0 {
    left: 0%;
  }
  .col-lg-offset-12 {
    margin-left: 100%;
  }
  .col-lg-offset-11 {
    margin-left: 91.66666666666666%;
  }
  .col-lg-offset-10 {
    margin-left: 83.33333333333334%;
  }
  .col-lg-offset-9 {
    margin-left: 75%;
  }
  .col-lg-offset-8 {
    margin-left: 66.66666666666666%;
  }
  .col-lg-offset-7 {
    margin-left: 58.333333333333336%;
  }
  .col-lg-offset-6 {
    margin-left: 50%;
  }
  .col-lg-offset-5 {
    margin-left: 41.66666666666667%;
  }
  .col-lg-offset-4 {
    margin-left: 33.33333333333333%;
  }
  .col-lg-offset-3 {
    margin-left: 25%;
  }
  .col-lg-offset-2 {
    margin-left: 16.666666666666664%;
  }
  .col-lg-offset-1 {
    margin-left: 8.333333333333332%;
  }
  .col-lg-offset-0 {
    margin-left: 0%;
  }
}
/*! normalize.css v3.0.0 | MIT License | git.io/normalize */
html {
  font-family: sans-serif;
  -ms-text-size-adjust: 100%;
  -webkit-text-size-adjust: 100%;
}
body {
  margin: 0;
}
article,
aside,
details,
figcaption,
figure,
footer,
header,
hgroup,
main,
nav,
section,
summary {
  display: block;
}
audio,
canvas,
progress,
video {
  display: inline-block;
  vertical-align: baseline;
}
audio:not([controls]) {
  display: none;
  height: 0;
}
[hidden],
template {
  display: none;
}
a {
  background: transparent;
}
a:active,
a:hover {
  outline: 0;
}
abbr[title] {
  border-bottom: 1px dotted;
}
b,
strong {
  font-weight: bold;
}
dfn {
  font-style: italic;
}
h1 {
  font-size: 2em;
  margin: 0.67em 0;
}
mark {
  background: #ff0;
  color: #000;
}
small {
  font-size: 80%;
}
sub,
sup {
  font-size: 75%;
  line-height: 0;
  position: relative;
  vertical-align: baseline;
}
sup {
  top: -0.5em;
}
sub {
  bottom: -0.25em;
}
img {
  border: 0;
}
svg:not(:root) {
  overflow: hidden;
}
figure {
  margin: 1em 40px;
}
hr {
  -moz-box-sizing: content-box;
  box-sizing: content-box;
  height: 0;
}
pre {
  overflow: auto;
}
code,
kbd,
pre,
samp {
  font-family: monospace, monospace;
  font-size: 1em;
}
button,
input,
optgroup,
select,
textarea {
  color: inherit;
  font: inherit;
  margin: 0;
}
button {
  overflow: visible;
}
button,
select {
  text-transform: none;
}
button,
html input[type="button"],
input[type="reset"],
input[type="submit"] {
  -webkit-appearance: button;
  cursor: pointer;
}
button[disabled],
html input[disabled] {
  cursor: default;
}
button::-moz-focus-inner,
input::-moz-focus-inner {
  border: 0;
  padding: 0;
}
input {
  line-height: normal;
}
input[type="checkbox"],
input[type="radio"] {
  box-sizing: border-box;
  padding: 0;
}
input[type="number"]::-webkit-inner-spin-button,
input[type="number"]::-webkit-outer-spin-button {
  height: auto;
}
input[type="search"] {
  -webkit-appearance: textfield;
  -moz-box-sizing: content-box;
  -webkit-box-sizing: content-box;
  box-sizing: content-box;
}
input[type="search"]::-webkit-search-cancel-button,
input[type="search"]::-webkit-search-decoration {
  -webkit-appearance: none;
}
fieldset {
  border: 1px solid #c0c0c0;
  margin: 0 2px;
  padding: 0.35em 0.625em 0.75em;
}
legend {
  border: 0;
  padding: 0;
}
textarea {
  overflow: auto;
}
optgroup {
  font-weight: bold;
}
table {
  border-collapse: collapse;
  border-spacing: 0;
}
td,
th {
  padding: 0;
}
@-ms-viewport {
  width: device-width;
}
.visible-xs,
.visible-sm,
.visible-md,
.visible-lg {
  display: none !important;
}
@media (max-width: 767px) {
  .visible-xs {
    display: block !important;
  }
  table.visible-xs {
    display: table;
  }
  tr.visible-xs {
    display: table-row !important;
  }
  th.visible-xs,
  td.visible-xs {
    display: table-cell !important;
  }
}
@media (min-width: 768px) and (max-width: 991px) {
  .visible-sm {
    display: block !important;
  }
  table.visible-sm {
    display: table;
  }
  tr.visible-sm {
    display: table-row !important;
  }
  th.visible-sm,
  td.visible-sm {
    display: table-cell !important;
  }
}
@media (min-width: 992px) and (max-width: 1199px) {
  .visible-md {
    display: block !important;
  }
  table.visible-md {
    display: table;
  }
  tr.visible-md {
    display: table-row !important;
  }
  th.visible-md,
  td.visible-md {
    display: table-cell !important;
  }
}
@media (min-width: 1200px) {
  .visible-lg {
    display: block !important;
  }
  table.visible-lg {
    display: table;
  }
  tr.visible-lg {
    display: table-row !important;
  }
  th.visible-lg,
  td.visible-lg {
    display: table-cell !important;
  }
}
@media (max-width: 767px) {
  .hidden-xs {
    display: none !important;
  }
}
@media (min-width: 768px) and (max-width: 991px) {
  .hidden-sm {
    display: none !important;
  }
}
@media (min-width: 992px) and (max-width: 1199px) {
  .hidden-md {
    display: none !important;
  }
}
@media (min-width: 1200px) {
  .hidden-lg {
    display: none !important;
  }
}
.visible-print {
  display: none !important;
}
@media print {
  .visible-print {
    display: block !important;
  }
  table.visible-print {
    display: table;
  }
  tr.visible-print {
    display: table-row !important;
  }
  th.visible-print,
  td.visible-print {
    display: table-cell !important;
  }
}
@media print {
  .hidden-print {
    display: none !important;
  }
}
* {
  -webkit-box-sizing: border-box;
  -moz-box-sizing: border-box;
  box-sizing: border-box;
}
*:before,
*:after {
  -webkit-box-sizing: border-box;
  -moz-box-sizing: border-box;
  box-sizing: border-box;
}
html {
  font-size: 62.5%;
  -webkit-tap-highlight-color: rgba(0, 0, 0, 0);
}
body {
  font-family: "Open Sans", Helvetica, Arial, sans-serif;
  font-size: 12px;
  line-height: 1.66666667;
  color: #333333;
  background-color: #ffffff;
}
input,
button,
select,
textarea {
  font-family: inherit;
  font-size: inherit;
  line-height: inherit;
}
a {
  color: #0099d3;
  text-decoration: none;
}
a:hover,
a:focus {
  color: #00618a;
  text-decoration: underline;
}
a:focus {
  outline: thin dotted;
  outline: 5px auto -webkit-focus-ring-color;
  outline-offset: -2px;
}
figure {
  margin: 0;
}
img {
  vertical-align: middle;
}
.img-responsive {
  display: block;
  max-width: 100%;
  height: auto;
}
.img-rounded {
  border-radius: 1px;
}
.img-thumbnail {
  padding: 4px;
  line-height: 1.66666667;
  background-color: #ffffff;
  border: 1px solid #dddddd;
  border-radius: 1px;
  -webkit-transition: all 0.2s ease-in-out;
  transition: all 0.2s ease-in-out;
  display: inline-block;
  max-width: 100%;
  height: auto;
}
.img-circle {
  border-radius: 50%;
}
hr {
  margin-top: 20px;
  margin-bottom: 20px;
  border: 0;
  border-top: 1px solid #eeeeee;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
}
.clearfix:before,
.clearfix:after,
.form-horizontal .form-group:before,
.form-horizontal .form-group:after,
.container:before,
.container:after,
.container-fluid:before,
.container-fluid:after,
.row:before,
.row:after {
  content: " ";
  display: table;
}
.clearfix:after,
.form-horizontal .form-group:after,
.container:after,
.container-fluid:after,
.row:after {
  clear: both;
}
.center-block {
  display: block;
  margin-left: auto;
  margin-right: auto;
}
.pull-right {
  float: right !important;
}
.pull-left {
  float: left !important;
}
.hide {
  display: none !important;
}
.show {
  display: block !important;
}
.invisible {
  visibility: hidden;
}
.text-hide {
  font: 0/0 a;
  color: transparent;
  text-shadow: none;
  background-color: transparent;
  border: 0;
}
.hidden {
  display: none !important;
  visibility: hidden !important;
}
.affix {
  position: fixed;
}
/* PatternFly specific */
/* Bootstrap overrides */
/* PatternFly-specific variables based on Bootstrap overides */
.alert {
  border-width: 2px;
  padding-left: 34px;
  position: relative;
}
.alert .alert-link {
  color: #0099d3;
}
.alert .alert-link:hover {
  color: #00618a;
}
.alert > .pficon,
.alert > .pficon-layered {
  font-size: 20px;
  position: absolute;
  left: 7px;
  top: 7px;
}
.alert .pficon-info {
  color: #72767b;
}
.alert-dismissable .close {
  right: -16px;
  top: 1px;
}
/* Bootstrap overrides */
/* PatternFly-specific */
.login-pf {
  height: 100%;
}
.login-pf #brand {
  position: relative;
  top: -70px;
}
.login-pf #brand img {
  display: block;
  height: 18px;
  margin: 0 auto;
  max-width: 100%;
}
@media (min-width: 768px) {
  .login-pf #brand img {
    margin: 0;
    text-align: left;
  }
}
.login-pf #badge {
  display: block;
  margin: 20px auto 70px;
  position: relative;
  text-align: center;
}
@media (min-width: 768px) {
  .login-pf #badge {
    float: right;
    margin-right: 64px;
    margin-top: 50px;
  }
}
.login-pf body {
  background: #1a1a1a url("../img/bg-login.png") repeat-x 50% 0;
  background-size: auto;
}
@media (min-width: 768px) {
  .login-pf body {
    background-size: 100% auto;
  }
}
.login-pf .container {
  background-color: transparent;
  clear: right;
  color: #fff;
  padding-bottom: 40px;
  padding-top: 20px;
  width: auto;
}
@media (min-width: 768px) {
  .login-pf .container {
    bottom: 13%;
    padding-left: 80px;
    position: absolute;
    width: 100%;
  }
}
.login-pf .container [class^='alert'] {
  background: transparent;
  color: #fff;
}
.login-pf .container .details p:first-child {
  border-top: 1px solid #474747;
  padding-top: 25px;
  margin-top: 25px;
}
@media (min-width: 768px) {
  .login-pf .container .details {
    border-left: 1px solid #474747;
    padding-left: 40px;
  }
  .login-pf .container .details p:first-child {
    border-top: 0;
    padding-top: 0;
    margin-top: 0;
  }
}
.login-pf .container .details p {
  margin-bottom: 2px;
}
.login-pf .container .form-horizontal .control-label {
  font-size: 13px;
  font-weight: 400;
  text-align: left;
}
.login-pf .container .form-horizontal .form-group:last-child,
.login-pf .container .form-horizontal .form-group:last-child .help-block:last-child {
  margin-bottom: 0;
}
.login-pf .container .help-block {
  color: #fff;
}
@media (min-width: 768px) {
  .login-pf .container .login {
    padding-right: 40px;
  }
}
.login-pf .container .submit {
  text-align: right;
}
.ie8.login-pf #badge {
  background: url('../img/logo.png') no-repeat;
  height: 44px;
  width: 137px;
}
.ie8.login-pf #badge img {
  width: 0;
}
.ie8.login-pf #brand {
  background: url('../img/brand-lg.png') no-repeat center;
  background-size: cover auto;
}
@media (min-width: 768px) {
  .ie8.login-pf #brand {
    background-position: 0 0;
  }
}
.ie8.login-pf #brand img {
  width: 0;
}
/* Bootstrap overrides */
/* RCUE-specific */
/* components/patternfly/less/login.less minus the font @import */
@font-face {
  font-family: 'PatternFlyIcons-webfont';
  src: url(data:application/font-woff;charset=utf-8;base64,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) format('woff');
  font-weight: normal;
  font-style: normal;
}
[class*="-exclamation"] {
  color: #fff;
}
[class^="pficon-"],
[class*=" pficon-"] {
  display: inline-block;
  font-family: 'PatternFlyIcons-webfont';
  font-style: normal;
  font-variant: normal;
  font-weight: normal;
  line-height: 1;
  speak: none;
  text-transform: none;
  /* Better Font Rendering =========== */
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
.pficon-layered {
  position: relative;
}
.pficon-layered .pficon:first-child {
  position: absolute;
  z-index: 1;
}
.pficon-layered .pficon:first-child + .pficon {
  position: relative;
  z-index: 2;
}
.pficon-warning-exclamation:before {
  content: "\e60d";
}
.pficon-screen:before {
  content: "\e600";
}
.pficon-save:before {
  content: "\e601";
}
.pficon-ok:before {
  color: #57a81c;
  content: "\e602";
}
.pficon-messages:before {
  content: "\e603";
}
.pficon-info:before {
  content: "\e604";
}
.pficon-help:before {
  content: "\e605";
}
.pficon-folder-open:before {
  content: "\e606";
}
.pficon-folder-close:before {
  content: "\e607";
}
.pficon-error-exclamation:before {
  content: "\e608";
}
.pficon-error-octagon:before {
  color: #c90813;
  content: "\e609";
}
.pficon-edit:before {
  content: "\e60a";
}
.pficon-close:before {
  content: "\e60b";
}
.pficon-warning-triangle:before {
  color: #eb7720;
  content: "\e60c";
}
.pficon-user:before {
  content: "\e60e";
}
.pficon-users:before {
  content: "\e60f";
}
.pficon-settings:before {
  content: "\e610";
}
.pficon-delete:before {
  content: "\e611";
}
.pficon-print:before {
  content: "\e612";
}
.pficon-refresh:before {
  content: "\e613";
}
.pficon-running:before {
  content: "\e614";
}
.pficon-import:before {
  content: "\e615";
}
.pficon-export:before {
  content: "\e616";
}
.pficon-history:before {
  content: "\e617";
}
.pficon-home:before {
  content: "\e618";
}
.pficon-remove:before {
  content: "\e619";
}
.pficon-add:before {
  content: "\e61a";
}
/* RCUE overrides */
.login-pf {
  background-color: #1a1a1a;
}
@media (min-width: 768px) {
  .login-pf {
    background-image: url("../img/bg-login-2.png");
    background-position: 100% 100%;
    background-repeat: no-repeat;
    background-size: 30%;
  }
}
@media (min-width: 992px) {
  .login-pf {
    background-size: auto;
  }
}
.login-pf #badge {
  margin-bottom: 50px;
}
.login-pf body {
  background: transparent;
}
@media (min-width: 768px) {
  .login-pf body {
    background-image: url("../img/bg-login.png");
    background-repeat: no-repeat;
    background-size: 30%;
    height: 100%;
  }
}
@media (min-width: 992px) {
  .login-pf body {
    background-size: auto;
  }
}
.login-pf #brand {
  top: -30px;
}
@media (min-width: 768px) {
  .login-pf #brand {
    top: -40px;
  }
  .login-pf #brand + .alert {
    margin-top: -20px;
  }
}
.login-pf .container {
  padding-top: 0;
}
@media (min-width: 992px) {
  .login-pf .container {
    bottom: 20%;
    padding-right: 120px;
  }
}
@media (min-width: 768px) {
  .login-pf {
    background-image: url(data:image/png;base64,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);
  }
  .login-pf body {
    background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAbwAAAG5CAMAAADCo3FeAAACuFBMVEUrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKyv///8rKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKysrKystLS0+Pj47OzvZvAriAAAA5HRSTlMAoGCQULCAcEDAINDgMPAQABXU0hQkCJzxQVH3GVgNB5OJ+h/j/KjFrPQtHKXEPIjKkt11t4LXmlrybJ0led/hZTjTTXdIUu9eIe7cMR1vsz92ybS4hA5XcRimfDY6U9t++/7OLrztpL4M53rI1gWuSgSHAbqXf/YKtuwPrd6UgeIzZisJS0IjYTJOY2QXo5seFm1zvWKM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);
  }
}
      </style>
  </head>
  <body>
    <span id="badge">
      <img src="data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4NCjwhLS0gR2VuZXJhdG9yOiBBZG9iZSBJbGx1c3RyYXRvciAxNi4wLjQsIFNWRyBFeHBvcnQgUGx1Zy1JbiAuIFNWRyBWZXJzaW9uOiA2LjAwIEJ1aWxkIDApICAtLT4NCjxzdmcgdmVyc2lvbj0iMS4yIiBiYXNlUHJvZmlsZT0idGlueSIgaWQ9ImxvZ28iIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgeG1sbnM6eGxpbms9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkveGxpbmsiDQoJIHg9IjBweCIgeT0iMHB4IiB3aWR0aD0iMTM3cHgiIGhlaWdodD0iNDRweCIgdmlld0JveD0iMCAwIDEzNyA0NCIgb3ZlcmZsb3c9InZpc2libGUiIHhtbDpzcGFjZT0icHJlc2VydmUiPg0KPGcgaWQ9ImxvZ29fMV8iPg0KCTxnIGlkPSJ3aGl0ZSI+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik00MC41MDUsMzMuOTMyYy0wLjg0Ni0wLjE5NS0xLjc0My0wLjMxNS0yLjY1MS0wLjMxNWMtMS41NTMsMC0yLjk2NSwwLjI2OC00LjAwOCwwLjcwNA0KCQkJYy0wLjExNCwwLjA1Ni0wLjE5NywwLjE3Ni0wLjE5NywwLjMxMmMwLDAuMDUsMC4wMTQsMC4xMDEsMC4wMzEsMC4xNDJjMC4xMjQsMC4zNTctMC4wNzksMC43NDUtMS4wODcsMC45NjYNCgkJCWMtMS40OTQsMC4zMjgtMi40NCwxLjg3My0yLjk3OSwyLjM4NGMtMC42MzYsMC42MDQtMi40MjYsMC45NzQtMi4xNTcsMC42MTRjMC4yMS0wLjI4LDEuMDE4LTEuMTU3LDEuNTA4LTIuMTAzDQoJCQljMC40MzgtMC44NDYsMC44MjktMS4wODcsMS4zNjctMS44OTRjMC4xNTYtMC4yMzYsMC43Ny0xLjA2OSwwLjk0Ny0xLjcyNWMwLjIwMS0wLjY0MywwLjEzNC0xLjQ0OCwwLjIxLTEuNzgNCgkJCWMwLjExLTAuNDgsMC41NjMtMS41MTcsMC41OTYtMi4xMDRjMC4wMjEtMC4zMzMtMS4zODQsMC40NzQtMi4wNSwwLjQ3NGMtMC42NjYsMC0xLjMxNS0wLjQtMS45MTItMC40MjgNCgkJCWMtMC43MzYtMC4wMzMtMS4yMDgsMC41NjgtMS44NzYsMC40NjNjLTAuMzc5LTAuMDYxLTAuNzAxLTAuMzk2LTEuMzY1LTAuNDIxYy0wLjk0OC0wLjAzNC0yLjEwNCwwLjUyNi00LjI3OSwwLjQ1Ng0KCQkJYy0yLjEzNy0wLjA2OC00LjExMy0yLjcwMi00LjM4Mi0zLjExOWMtMC4zMTUtMC40OTEtMC43MDItMC40OTEtMS4xMjItMC4xMDdjLTAuNDIxLDAuMzg3LTAuOTM5LDAuMDg0LTEuMDg3LTAuMTc2DQoJCQljLTAuMjc5LTAuNDg5LTEuMDI5LTEuOTIyLTIuMTkxLTIuMjI2Yy0xLjYwNS0wLjQxNi0yLjQxOSwwLjg5LTIuMzEyLDEuOTI5YzAuMTA3LDEuMDU0LDAuNzg3LDEuMzQ5LDEuMTA0LDEuOTEyDQoJCQljMC4zMTUsMC41NTksMC40NzcsMC45MTksMS4wNywxLjE2OGMwLjQyMSwwLjE3NywwLjU3NywwLjQzNywwLjQ1MywwLjc4Yy0wLjExMSwwLjMwMS0wLjU0OSwwLjM3MS0wLjgzNywwLjM4NA0KCQkJYy0wLjYxLDAuMDMtMS4wNC0wLjEzNi0xLjM1NC0wLjMzNmMtMC4zNjMtMC4yMzEtMC42NTktMC41NTMtMC45NzgtMS4xYy0wLjM2OC0wLjYwNC0wLjk0NS0wLjg2Ni0xLjYyLTAuODY2DQoJCQljLTAuMzIxLDAtMC42MjEsMC4wODQtMC44ODksMC4yMjFjLTEuMDU3LDAuNTUyLTIuMzEzLDAuODc3LTMuNjY3LDAuODc3SDEuMjY3QzQuMTk1LDM3LjcsMTIuNDA1LDQzLjk1MywyMi4wNzQsNDMuOTUzDQoJCQlDMjkuOCw0My45NTEsMzYuNTk0LDM5Ljk2Niw0MC41MDUsMzMuOTMyeiIvPg0KCTwvZz4NCgk8ZyBpZD0iYmxhY2siPg0KCQk8Zz4NCgkJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik00My44MDcsMzIuNzYxaDAuMTk4bDAuMjk5LDAuNDloMC4xOTJsLTAuMzIzLTAuNTAxYzAuMTY4LTAuMDIxLDAuMjk1LTAuMTA4LDAuMjk1LTAuMzA5DQoJCQkJYzAtMC4yMjYtMC4xMzMtMC4zMjQtMC40LTAuMzI0aC0wLjQzMXYxLjEzNGgwLjE3MUw0My44MDcsMzIuNzYxTDQzLjgwNywzMi43NjF6IE00My44MDcsMzIuNjE1di0wLjM1MmgwLjIzMw0KCQkJCWMwLjExOSwwLDAuMjQ3LDAuMDI1LDAuMjQ3LDAuMTY1YzAsMC4xNzYtMC4xMjksMC4xODctMC4yNzMsMC4xODdINDMuODA3TDQzLjgwNywzMi42MTV6Ii8+DQoJCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNNDUuMTE5LDMyLjY4NmMwLDAuNjExLTAuNDk2LDEuMTEtMS4xMDgsMS4xMWMtMC42MTQsMC0xLjExLTAuNDk5LTEuMTEtMS4xMXMwLjQ5Ni0xLjExLDEuMTEtMS4xMQ0KCQkJCUM0NC42MjMsMzEuNTc1LDQ1LjExOSwzMi4wNzQsNDUuMTE5LDMyLjY4NnogTTQ0LjAxMSwzMS43NzNjLTAuNTA1LDAtMC45MTUsMC40MDktMC45MTUsMC45MTJjMCwwLjUwNSwwLjQxLDAuOTEyLDAuOTE1LDAuOTEyDQoJCQkJYzAuNTAzLDAsMC45MTMtMC40MDcsMC45MTMtMC45MTJDNDQuOTIyLDMyLjE4Myw0NC41MTQsMzEuNzczLDQ0LjAxMSwzMS43NzN6Ii8+DQoJCTwvZz4NCgkJPGc+DQoJCQk8cGF0aCBkPSJNNDAuNTA1LDMzLjkzNGMtMC44NDYtMC4xOTQtMS43NDMtMC4zMTUtMi42NTEtMC4zMTVjLTEuNTUzLDAtMi45NjUsMC4yNy00LjAwOCwwLjcwMg0KCQkJCWMtMC4xMTQsMC4wNTgtMC4xOTcsMC4xNzYtMC4xOTcsMC4zMTRjMCwwLjA0OCwwLjAxNCwwLjEwMSwwLjAzMSwwLjE0MmMwLjEyNCwwLjM1Ny0wLjA3OSwwLjc0NS0xLjA4NywwLjk2Nw0KCQkJCWMtMS40OTQsMC4zMjctMi40NCwxLjg3LTIuOTc5LDIuMzgzYy0wLjYzNiwwLjYwMi0yLjQyNiwwLjk3NC0yLjE1NywwLjYxNGMwLjIxLTAuMjgsMS4wMTgtMS4xNTcsMS41MDgtMi4xMDINCgkJCQljMC40MzgtMC44NDUsMC44MjktMS4wODgsMS4zNjctMS44OTNjMC4xNTYtMC4yMzgsMC43Ny0xLjA2OSwwLjk0Ny0xLjcyN2MwLjIwMS0wLjY0MywwLjEzNC0xLjQ0OCwwLjIxLTEuNzgNCgkJCQljMC4xMS0wLjQ4LDAuNTYzLTEuNTE3LDAuNTk2LTIuMTA0YzAuMDIxLTAuMzMyLTEuMzg0LDAuNDcyLTIuMDUsMC40NzJjLTAuNjY2LDAtMS4zMTUtMC4zOTgtMS45MTItMC40MjYNCgkJCQljLTAuNzM2LTAuMDM1LTEuMjA4LDAuNTY4LTEuODc2LDAuNDYxYy0wLjM3OS0wLjA2MS0wLjcwMS0wLjM5NS0xLjM2NS0wLjQyMWMtMC45NDgtMC4wMzQtMi4xMDQsMC41MjgtNC4yNzksMC40NTYNCgkJCQljLTIuMTM3LTAuMDY4LTQuMTEzLTIuNy00LjM4Mi0zLjExN2MtMC4zMTUtMC40OTMtMC43MDItMC40OTMtMS4xMjItMC4xMDdjLTAuNDIxLDAuMzg1LTAuOTM5LDAuMDgyLTEuMDg3LTAuMTc2DQoJCQkJYy0wLjI3OS0wLjQ5MS0xLjAyOS0xLjkyNC0yLjE5MS0yLjIyNWMtMS42MDUtMC40MTktMi40MTksMC44ODktMi4zMTIsMS45MjhjMC4xMDcsMS4wNTIsMC43ODcsMS4zNDcsMS4xMDQsMS45MDgNCgkJCQljMC4zMTUsMC41NjEsMC40NzcsMC45MjMsMS4wNywxLjE3M2MwLjQyMSwwLjE3NCwwLjU3NywwLjQzNiwwLjQ1MywwLjc3OWMtMC4xMTEsMC4zMDEtMC41NDksMC4zNjktMC44MzcsMC4zODQNCgkJCQljLTAuNjEsMC4wMy0xLjA0LTAuMTM4LTEuMzU0LTAuMzM4Yy0wLjM2My0wLjIzMS0wLjY1OS0wLjU1My0wLjk3OC0xLjFjLTAuMzY4LTAuNjA0LTAuOTQ1LTAuODY2LTEuNjItMC44NjYNCgkJCQljLTAuMzIxLDAtMC42MjEsMC4wODYtMC44ODksMC4yMjFjLTEuMDU3LDAuNTUyLTIuMzEzLDAuODc3LTMuNjY3LDAuODc3SDEuMjY3Yy0wLjc0My0yLjIwMy0xLjE0Ni00LjU2NC0xLjE0Ni03LjAxNw0KCQkJCWMwLTEyLjEyNyw5LjgzLTIxLjk1NSwyMS45NTMtMjEuOTU1YzEyLjEyNSwwLDIxLjk1NCw5LjgyOCwyMS45NTQsMjEuOTU1QzQ0LjAyOCwyNi40LDQyLjczNCwzMC40OTYsNDAuNTA1LDMzLjkzNHoiLz4NCgkJPC9nPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNNTUuMjE5LDIzLjM4NGMwLTIuMDEyLTAuMDQyLTMuNDk0LTAuMTIyLTQuODMyaDMuMjkzbDAuMTQsMi44NTVoMC4xMDgNCgkJCWMwLjczOS0yLjExNiwyLjQ5NC0zLjE5Niw0LjExNy0zLjE5NmMwLjM3MiwwLDAuNTg2LDAuMDEzLDAuODkyLDAuMDgydjMuNTgyYy0wLjM1Ni0wLjA2OC0wLjY4OC0wLjEwOC0xLjE0Ni0wLjEwOA0KCQkJYy0xLjgxMywwLTMuMDcsMS4xNTQtMy40MDksMi44NzhjLTAuMDY0LDAuMzMzLTAuMDk5LDAuNzM2LTAuMDk5LDEuMTQ2djcuODAxaC0zLjgwNUw1NS4yMTksMjMuMzg0eiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNNjguMjM5LDI3LjA5NmMwLjEwMSwyLjcyNiwyLjIxLDMuOTE4LDQuNjQ2LDMuOTE4YzEuNzQ4LDAsMy4wMDEtMC4yNzMsNC4xNTEtMC42OTdsMC41NjEsMi42MTcNCgkJCWMtMS4yODUsMC41NDctMy4wNzEsMC45NTQtNS4yNTUsMC45NTRjLTQuODg1LDAtNy43NDYtMy4wMTYtNy43NDYtNy42MjRjMC00LjE0OSwyLjUxOC04LjA3OSw3LjM1OC04LjA3OQ0KCQkJYzQuODksMCw2LjQ4LDQuMDIyLDYuNDgsNy4zMTVjMCwwLjcwOC0wLjA2MywxLjI3My0wLjEzNCwxLjYyNUw2OC4yMzksMjcuMDk2eiBNNzQuODUyLDI0LjQ0NQ0KCQkJYzAuMDE5LTEuMzk0LTAuNTg4LTMuNjY0LTMuMTM2LTMuNjY0Yy0yLjMzOSwwLTMuMzE0LDIuMTI1LTMuNDg2LDMuNjY0SDc0Ljg1MnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTkxLjAxOSwyNy4wNjljMCwwLjM5OC0wLjAyNiwwLjc3LTAuMTE0LDEuMTFjLTAuMzgzLDEuNjQ2LTEuNzMsMi43MDctMy4yODMsMi43MDcNCgkJCWMtMi4zOTYsMC0zLjc2NS0yLjAyMS0zLjc2NS00Ljc4M2MwLTIuNzksMS4zNTgtNC45NSwzLjgwNy00Ljk1YzEuNzExLDAsMi45MzUsMS4yMDcsMy4yNywyLjY3MQ0KCQkJYzAuMDY1LDAuMzA2LDAuMDg2LDAuNjg4LDAuMDg2LDAuOTlWMjcuMDY5TDkxLjAxOSwyNy4wNjl6IE05NC44MTksMTIuNzYxbC0zLjgwMS0xLjA3MnY4LjQ4NGgtMC4wNjQNCgkJCWMtMC42NzItMS4xMTItMi4xNTYtMS45NTktNC4yMTQtMS45NTljLTMuNjE3LDAtNi43NjcsMi45OTMtNi43NDIsOC4wMzJjMCw0LjYyMiwyLjg0NCw3LjY4Nyw2LjQzNiw3LjY4Nw0KCQkJYzIuMTcsMCwzLjk4My0xLjAzNSw0Ljg4NC0yLjcxOWgwLjA2NmwwLjE3LDIuMzgyaDMuMzg4Yy0wLjA3LTEuMDIyLTAuMTI0LTIuNjc5LTAuMTI0LTQuMjE4VjEyLjc2MUg5NC44MTl6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xMDQuODc2LDE4LjE5NmMtMS4xNDUsMC0yLjE3MiwwLjMzLTMuMDM0LDAuODYyYy0wLjg5NSwwLjUyNC0xLjYyMiwxLjMzMy0yLjA1NiwyLjE3aC0wLjA2MXYtNy4wMzkNCgkJCWwtMS40OTEtMC40NHYxOS44NDNoMS40OTF2LTkuMjA1YzAtMC42MTIsMC4wNDctMS4wMzYsMC4yMDItMS40ODFjMC42NDQtMS44NzUsMi40MDctMy40MTEsNC41NC0zLjQxMQ0KCQkJYzMuMDgxLDAsNC4xNDgsMi40NzEsNC4xNDgsNS4xODJ2OC45MTVoMS40ODh2LTkuMDc4QzExMC4xMDQsMTguOTA3LDEwNi4zMDIsMTguMTk2LDEwNC44NzYsMTguMTk2eiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMTIzLjQ2NSwzMC4wMTZjMCwxLjE5LDAuMDQ5LDIuNDI0LDAuMjIyLDMuNTc1aC0xLjM3MWwtMC4yMTktMi4xNTdoLTAuMDcxDQoJCQljLTAuNzMsMS4xNi0yLjQwNiwyLjUtNC43OTgsMi41Yy0zLjAyOCwwLTQuNDM4LTIuMTMxLTQuNDM4LTQuMTM5YzAtMy40NzMsMy4wNjctNS41NjYsOS4xOTEtNS41MDJ2LTAuNDAyDQoJCQljMC0xLjQ4OS0wLjI5LTQuNDU4LTMuODUyLTQuNDM1Yy0xLjMxNiwwLTIuNjksMC4zNTMtMy43NzcsMS4xMjFsLTAuNDc0LTEuMDgzYzEuMzc2LTAuOTMxLDMuMDUzLTEuMyw0LjQxNC0xLjMNCgkJCWM0LjM0NiwwLDUuMTc1LDMuMjYyLDUuMTc1LDUuOTV2NS44NzJIMTIzLjQ2NXogTTEyMS45NzksMjUuNTQ2Yy0zLjI3Ny0wLjA5NC03LjYwOCwwLjQwMi03LjYwOCw0LjAxOA0KCQkJYzAsMi4xNjMsMS40MjgsMy4xMzYsMi45OTYsMy4xMzZjMi41MSwwLDMuOTM1LTEuNTU0LDQuNDU1LTMuMDJjMC4xMDctMC4zMjEsMC4xNTctMC42NDUsMC4xNTctMC45VjI1LjU0NkwxMjEuOTc5LDI1LjU0NnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTEyOS4zOTMsMTUuMjIydjMuMzE3aDQuMjkxdjEuMjA4aC00LjI5MXY5Ljc4M2MwLDEuOTE1LDAuNTk0LDMuMTEyLDIuMjEyLDMuMTEyDQoJCQljMC43NzcsMCwxLjMyMy0wLjEsMS43MS0wLjIzNmwwLjE4MiwxLjE1NWMtMC40ODYsMC4yMDItMS4xNywwLjM2LTIuMDc5LDAuMzZjLTEuMDk3LDAtMi4wMDgtMC4zNDYtMi41OTYtMS4wNjgNCgkJCWMtMC42ODQtMC43OTEtMC45MTctMi4wNTQtMC45MTctMy41OXYtOS41MTdoLTIuNTR2LTEuMjA4aDIuNTR2LTIuNzY3TDEyOS4zOTMsMTUuMjIyeiIvPg0KCQk8Zz4NCgkJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xMzUuNTY0LDMyLjc5MWgwLjE5OGwwLjI5OCwwLjQ4OWgwLjE5M2wtMC4zMjItMC41MDJjMC4xNjYtMC4wMTgsMC4yOTQtMC4xMDUsMC4yOTQtMC4zMDgNCgkJCQljMC0wLjIyNi0wLjEzNC0wLjMyNC0wLjQwMS0wLjMyNGgtMC40M3YxLjEzNGgwLjE3MnYtMC40ODlIMTM1LjU2NHogTTEzNS41NjQsMzIuNjQ0di0wLjM1MWgwLjIzMg0KCQkJCWMwLjExOCwwLDAuMjQ3LDAuMDI1LDAuMjQ3LDAuMTY1YzAsMC4xNzUtMC4xMjksMC4xODYtMC4yNzUsMC4xODZIMTM1LjU2NEwxMzUuNTY0LDMyLjY0NHoiLz4NCgkJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xMzYuODc5LDMyLjcxNWMwLDAuNjE0LTAuNDk5LDEuMTA5LTEuMTEsMS4xMDlzLTEuMTEtMC40OTUtMS4xMS0xLjEwOWMwLTAuNjExLDAuNDk5LTEuMTA5LDEuMTEtMS4xMDkNCgkJCQlDMTM2LjM4MSwzMS42MDUsMTM2Ljg3OSwzMi4xMDQsMTM2Ljg3OSwzMi43MTV6IE0xMzUuNzY5LDMxLjgwMWMtMC41MDUsMC0wLjkxNCwwLjQxMS0wLjkxNCwwLjkxNA0KCQkJCWMwLDAuNTAyLDAuNDA5LDAuOTA5LDAuOTE0LDAuOTA5YzAuNTAzLDAsMC45MTEtMC40MDcsMC45MTEtMC45MDlDMTM2LjY4LDMyLjIxMiwxMzYuMjcxLDMxLjgwMSwxMzUuNzY5LDMxLjgwMXoiLz4NCgkJPC9nPg0KCQk8cGF0aCBkPSJNMjYuOTEsMzEuOTE5YzAuMTEzLDAuMTExLDAuMzA4LDAuNDgzLDAuMDcsMC45NTJjLTAuMTM0LDAuMjQ5LTAuMjc3LDAuNDIzLTAuNTM0LDAuNjMxDQoJCQljLTAuMzA5LDAuMjQ1LTAuOTEzLDAuNTMxLTEuNzQyLDAuMDA4Yy0wLjQ0NC0wLjI4My0wLjQ3MS0wLjM3OS0xLjA4NS0wLjI5OGMtMC40NCwwLjA1Ny0wLjYxNC0wLjM4Ni0wLjQ1Ny0wLjc1Ng0KCQkJYzAuMTU4LTAuMzY3LDAuODA3LTAuNjY2LDEuNjE0LTAuMTkyYzAuMzYyLDAuMjE0LDAuOTI3LDAuNjY0LDEuNDIzLDAuMjY2YzAuMjA2LTAuMTY2LDAuMzI3LTAuMjczLDAuNjEzLTAuNjA0DQoJCQljMC4wMTMtMC4wMTUsMC4wMy0wLjAyMywwLjA1MS0wLjAyM0MyNi44ODEsMzEuOTAyLDI2Ljg5OCwzMS45MTEsMjYuOTEsMzEuOTE5eiIvPg0KCTwvZz4NCgk8cGF0aCBpZD0icmVkIiBmaWxsPSIjQ0MwMDAwIiBkPSJNMjAuMzY3LDEwLjUwMWMtMi41MzYsMC4xODMtMi43OTksMC40NTctMy4yNzQsMC45NjJjLTAuNjcsMC43MTMtMS41NTMtMC45MjUtMS41NTMtMC45MjUNCgkJYy0wLjUyOS0wLjExMS0xLjE3MS0wLjk2NS0wLjgyNS0xLjc2M2MwLjM0LTAuNzg4LDAuOTctMC41NTIsMS4xNjktMC4zMDZjMC4yNCwwLjI5OSwwLjc1MywwLjc4NywxLjQxOSwwLjc3MQ0KCQljMC42NjUtMC4wMTgsMS40MzQtMC4xNTgsMi41MDQtMC4xNThjMS4wODUsMCwxLjgxNSwwLjQwNSwxLjg1NSwwLjc1M0MyMS42OTgsMTAuMTM0LDIxLjU3NiwxMC40MTMsMjAuMzY3LDEwLjUwMXogTTIzLjAzLDYuMzENCgkJYy0wLjAwMywwLTAuMDA3LDAtMC4wMTIsMGMtMC4wMzksMC0wLjA3MS0wLjAyOS0wLjA3MS0wLjA2NWMwLTAuMDI2LDAuMDE3LTAuMDQ5LDAuMDQxLTAuMDYxYzAuNDkyLTAuMjU5LDEuMjI1LTAuNDY3LDIuMDY0LTAuNTUxDQoJCWMwLjI1Mi0wLjAyOCwwLjQ5OS0wLjA0LDAuNzM1LTAuMDQyYzAuMDQyLDAsMC4wODQsMCwwLjEyNiwwYzEuNDA4LDAuMDMyLDIuNTMzLDAuNTksMi41MTYsMS4yNDgNCgkJYy0wLjAxNywwLjY1OS0xLjE2OSwxLjE2Ny0yLjU3NywxLjEzNWMtMC40NTUtMC4wMTEtMC44ODMtMC4wNzctMS4yNS0wLjE4M2MtMC4wNDQtMC4wMTEtMC4wNzUtMC4wNDktMC4wNzUtMC4wOTENCgkJYzAtMC4wNDQsMC4wMzEtMC4wODMsMC4wNzUtMC4wOTRjMC44NzktMC4yMDMsMS40NzItMC41MzUsMS40My0wLjg0OWMtMC4wNTQtMC40MTYtMS4yMDQtMC42NDMtMi41NjUtMC41MDUNCgkJQzIzLjMxNiw2LjI2OSwyMy4xNzIsNi4yODgsMjMuMDMsNi4zMXogTTM0LjQ2NCwxNi4xNmMtMC4yMTgsMC43MjktMC41MjYsMS42NjEtMS44OTgsMi4zNjdjLTAuMiwwLjEwMi0wLjI3Ny0wLjA2NS0wLjE4NC0wLjIyMw0KCQljMC41MTgtMC44ODIsMC42MTEtMS4xMDQsMC43NjEtMS40NDljMC4yMTEtMC41MTEsMC4zMjEtMS4yMzMtMC4wOTgtMi43NDRjLTAuODI3LTIuOTcyLTIuNTQ4LTYuOTQ0LTMuOC04LjIzMQ0KCQljLTEuMjA4LTEuMjQ0LTMuMzk3LTEuNTk0LTUuMzc2LTEuMDg2Yy0wLjczLDAuMTg4LTIuMTU0LDAuOTI5LTQuNzk5LDAuMzM0Yy00LjU3Ni0xLjAzMS01LjI1NCwxLjI2MS01LjUxNiwyLjI1OA0KCQljLTAuMjYxLDAuOTk5LTAuODkzLDMuODM1LTAuODkzLDMuODM1Yy0wLjIxLDEuMTU2LTAuNDg1LDMuMTY1LDYuNjIsNC41MmMzLjMxLDAuNjI4LDMuNDc4LDEuNDg0LDMuNjIzLDIuMQ0KCQljMC4yNjQsMS4xMDQsMC42ODMsMS43MzQsMS4xNTcsMi4wNDljMC40NzMsMC4zMTcsMCwwLjU3OS0wLjUyNCwwLjYzYy0xLjQwOSwwLjE0Ny02LjYyLTEuMzQ2LTkuNzAyLTMuMDk4DQoJCWMtMi41MjItMS41NDEtMi41NjQtMi45MjktMS45ODctNC4xMDVjLTMuODA5LTAuNDExLTYuNjY5LDAuMzU3LTcuMTg2LDIuMTZjLTAuODkyLDMuMDk1LDYuODAzLDguMzgxLDE1LjU2MywxMS4wMzQNCgkJYzkuMTk1LDIuNzg1LDE4LjY1MSwwLjg0MiwxOS43MDMtNC45MzdDNDAuNDA0LDE4Ljk0MywzOC4xOTIsMTcsMzQuNDY0LDE2LjE2eiIvPg0KPC9nPg0KPC9zdmc+DQo=" alt="Red Hat&reg; logo" />
    </span>
    <div class="container">
      <div class="row">
        <div class="col-sm-12">
          <div id="brand">
            <img src="data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4NCjwhLS0gR2VuZXJhdG9yOiBBZG9iZSBJbGx1c3RyYXRvciAxNi4wLjQsIFNWRyBFeHBvcnQgUGx1Zy1JbiAuIFNWRyBWZXJzaW9uOiA2LjAwIEJ1aWxkIDApICAtLT4NCjwhRE9DVFlQRSBzdmcgUFVCTElDICItLy9XM0MvL0RURCBTVkcgMS4xLy9FTiIgImh0dHA6Ly93d3cudzMub3JnL0dyYXBoaWNzL1NWRy8xLjEvRFREL3N2ZzExLmR0ZCI+DQo8c3ZnIHZlcnNpb249IjEuMSIgaWQ9IkxheWVyXzEiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgeG1sbnM6eGxpbms9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkveGxpbmsiIHg9IjBweCIgeT0iMHB4Ig0KCSB3aWR0aD0iMjE1cHgiIGhlaWdodD0iMTBweCIgdmlld0JveD0iMCAwIDIxNSAxMCIgZW5hYmxlLWJhY2tncm91bmQ9Im5ldyAwIDAgMjE1IDEwIiB4bWw6c3BhY2U9InByZXNlcnZlIj4NCjxnPg0KCTxnPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNNjcuNjcsOS45MTVjLTIuMzIzLDAtMy42OTctMS44LTMuNjk3LTQuMzkyYzAtMi41ODksMS4zNzQtNC4zOSwzLjY5Ny00LjM5YzIuMzIzLDAsMy42OTcsMS44LDMuNjk3LDQuMzkNCgkJCUM3MS4zNjcsOC4xMTUsNjkuOTkzLDkuOTE1LDY3LjY3LDkuOTE1eiBNNjcuNjcsMi44MjVjLTEuMzc1LDAtMS45NDYsMS4xOC0xLjk0NiwyLjY5OGMwLDEuNTIxLDAuNTcxLDIuNzAxLDEuOTQ2LDIuNzAxDQoJCQljMS4zNzQsMCwxLjk0Ni0xLjE4MSwxLjk0Ni0yLjcwMUM2OS42MTYsNC4wMDUsNjkuMDQ0LDIuODI1LDY3LjY3LDIuODI1eiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNNzYuNjgsNi43NjZoLTEuODczdjMuMDE3aC0xLjcwMlYxLjI2OWgzLjcyMWMxLjYwNSwwLDIuOTMxLDAuODg4LDIuOTMxLDIuNw0KCQkJQzc5Ljc1Nyw1LjkzOCw3OC40NDMsNi43NjYsNzYuNjgsNi43NjZ6IE03Ni43NjUsMi45MjJoLTEuOTU4djIuMTg5aDEuOTgyYzAuNzkxLDAsMS4yMTYtMC4zNjUsMS4yMTYtMS4xMDYNCgkJCUM3OC4wMDYsMy4yNjMsNzcuNTIsMi45MjIsNzYuNzY1LDIuOTIyeiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNODEuMzc0LDkuNzgxVjEuMjY5aDUuOTF2MS42NjZoLTQuMjA4djEuNDcxaDIuNDQ1djEuNjU0aC0yLjQ0NXYyLjA1Nmg0LjM5MXYxLjY2Nkw4MS4zNzQsOS43ODENCgkJCUw4MS4zNzQsOS43ODF6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik05NC42NTIsOS43ODFsLTMuMTI1LTQuNjQ2Yy0wLjIwNy0wLjMxNi0wLjQ4Ni0wLjc0Mi0wLjU5Ni0wLjk2MWMwLDAuMzE2LDAuMDI0LDEuMzg3LDAuMDI0LDEuODZ2My43NDUNCgkJCWgtMS42Nzh2LTguNTFoMS42MjlsMy4wMTYsNC40OTljMC4yMDcsMC4zMTYsMC40ODYsMC43NDIsMC41OTYsMC45NjFjMC0wLjMxNS0wLjAyNC0xLjM4Ni0wLjAyNC0xLjg1OXYtMy42aDEuNjc4djguNTEzDQoJCQlMOTQuNjUyLDkuNzgxTDk0LjY1Miw5Ljc4MXoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTEwMS4yNTUsOS45MTVjLTEuNDIzLDAtMi42NjMtMC41OTctMy4zMDgtMS41NDVsMS4yMjgtMS4wOTVjMC41OTYsMC42OTIsMS4zNjIsMC45NzQsMi4xNzcsMC45NzQNCgkJCWMxLjAwOSwwLDEuNDgzLTAuMjgsMS40ODMtMC45MjVjMC0wLjU0Ny0wLjI5Mi0wLjc5LTEuODk3LTEuMTU1Yy0xLjU2OS0wLjM2NC0yLjY2NC0wLjg2My0yLjY2NC0yLjU0Mg0KCQkJYzAtMS41NDQsMS4zNjItMi40OTMsMy4wNDEtMi40OTNjMS4zMjYsMCwyLjI5OCwwLjQ5OSwzLjEwMSwxLjMzN2wtMS4yMjksMS4xOTJjLTAuNTQ3LTAuNTYtMS4xNTUtMC44NzUtMS45MzQtMC44NzUNCgkJCWMtMC45MTIsMC0xLjIxNiwwLjM4OS0xLjIxNiwwLjc2NmMwLDAuNTM1LDAuMzY1LDAuNzA2LDEuNzE0LDEuMDIxYzEuMzUsMC4zMTYsMi44NDYsMC43NzgsMi44NDYsMi42MjcNCgkJCUMxMDQuNTk5LDguODIsMTAzLjU3OCw5LjkxNSwxMDEuMjU1LDkuOTE1eiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMTExLjU5Miw5Ljc4MVY2LjIwNmgtMy40OXYzLjU3NWgtMS43MDNWMS4yNjloMS43MDN2My4yNTloMy40OVYxLjI2OWgxLjcwMnY4LjUxM0wxMTEuNTkyLDkuNzgxDQoJCQlMMTExLjU5Miw5Ljc4MXoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTExNS42MDQsOS43ODFWMS4yNjloMS43MDN2OC41MTNMMTE1LjYwNCw5Ljc4MUwxMTUuNjA0LDkuNzgxeiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMTIxLjMxOCwyLjkzNVY0LjU0aDIuNTY1djEuNjUzaC0yLjU2NXYzLjU4OWgtMS43MDJWMS4yNjloNi4wOTN2MS42NjZIMTIxLjMxOHoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTEzMC44MTYsMi45NDd2Ni44MzRoLTEuNzAzVjIuOTQ3aC0yLjQ0NFYxLjI2OWg2LjU5MnYxLjY3OEgxMzAuODE2eiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMTM4LjQyOCw5Ljc4MVYxLjI2OWg1LjkxdjEuNjY2aC00LjIwOHYxLjQ3MWgyLjQ0NHYxLjY1NGgtMi40NDR2Mi4wNTZoNC4zOTJ2MS42NjZMMTM4LjQyOCw5Ljc4MQ0KCQkJTDEzOC40MjgsOS43ODF6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xNTEuNzA3LDkuNzgxbC0zLjEyNS00LjY0NmMtMC4yMDctMC4zMTYtMC40ODYtMC43NDItMC41OTgtMC45NjFjMCwwLjMxNiwwLjAyNCwxLjM4NywwLjAyNCwxLjg2djMuNzQ1DQoJCQloLTEuNjc4di04LjUxaDEuNjNsMy4wMTYsNC40OTljMC4yMDcsMC4zMTYsMC40ODYsMC43NDIsMC41OTcsMC45NjFjMC0wLjMxNS0wLjAyNC0xLjM4Ni0wLjAyNC0xLjg1OXYtMy42aDEuNjh2OC41MTMNCgkJCUwxNTEuNzA3LDkuNzgxTDE1MS43MDcsOS43ODF6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xNTkuMDE2LDIuOTQ3djYuODM0aC0xLjcwM1YyLjk0N2gtMi40NDRWMS4yNjloNi41OTJ2MS42NzhIMTU5LjAxNnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTE2My4xMDIsOS43ODFWMS4yNjloNS45MXYxLjY2NmgtNC4yMDl2MS40NzFoMi40NDR2MS42NTRoLTIuNDQ0djIuMDU2aDQuMzkxdjEuNjY2TDE2My4xMDIsOS43ODENCgkJCUwxNjMuMTAyLDkuNzgxeiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMTc1Ljk0MSw5Ljc4MWwtMS41MjEtMy4wNjRoLTEuNzE1djMuMDY0aC0xLjcwMlYxLjI2OWgzLjk2NGMxLjYwNSwwLDIuOTMyLDAuODg4LDIuOTMyLDIuNw0KCQkJYzAsMS4yNzctMC41NDcsMi4wOC0xLjYyOSwyLjUwNmwxLjYyOSwzLjMwOEwxNzUuOTQxLDkuNzgxTDE3NS45NDEsOS43ODF6IE0xNzQuOTMyLDIuOTIyaC0yLjIyNnYyLjE4OWgyLjIyNg0KCQkJYzAuNzkxLDAsMS4yMTctMC4zNjUsMS4yMTctMS4xMDZDMTc2LjE0NiwzLjIzOCwxNzUuNjg2LDIuOTIyLDE3NC45MzIsMi45MjJ6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0xODMuMjYyLDYuNzY2aC0xLjg3M3YzLjAxN2gtMS43MDFWMS4yNjloMy43MjFjMS42MDUsMCwyLjkzMiwwLjg4OCwyLjkzMiwyLjcNCgkJCUMxODYuMzM5LDUuOTM4LDE4NS4wMjUsNi43NjYsMTgzLjI2Miw2Ljc2NnogTTE4My4zNDgsMi45MjJoLTEuOTU5djIuMTg5aDEuOTgyYzAuNzkxLDAsMS4yMTYtMC4zNjUsMS4yMTYtMS4xMDYNCgkJCUMxODQuNTg3LDMuMjYzLDE4NC4xMDIsMi45MjIsMTgzLjM0OCwyLjkyMnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTE5Mi44OTMsOS43ODFsLTEuNTIxLTMuMDY0aC0xLjcxNXYzLjA2NGgtMS43MDJWMS4yNjloMy45NjRjMS42MDQsMCwyLjkzMywwLjg4OCwyLjkzMywyLjcNCgkJCWMwLDEuMjc3LTAuNTQ5LDIuMDgtMS42MzEsMi41MDZsMS42MzEsMy4zMDhMMTkyLjg5Myw5Ljc4MUwxOTIuODkzLDkuNzgxeiBNMTkxLjg4MywyLjkyMmgtMi4yMjZ2Mi4xODloMi4yMjYNCgkJCWMwLjc5MSwwLDEuMjE3LTAuMzY1LDEuMjE3LTEuMTA2QzE5My4xLDMuMjM4LDE5Mi42MzcsMi45MjIsMTkxLjg4MywyLjkyMnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTE5Ni43NTgsOS43ODFWMS4yNjloMS43MDN2OC41MTNMMTk2Ljc1OCw5Ljc4MUwxOTYuNzU4LDkuNzgxeiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNMjAzLjY2Niw5LjkxNWMtMS40MjMsMC0yLjY2NC0wLjU5Ny0zLjMwOS0xLjU0NWwxLjIyOS0xLjA5NWMwLjU5NiwwLjY5MiwxLjM2MiwwLjk3NCwyLjE3OCwwLjk3NA0KCQkJYzEuMDEsMCwxLjQ4My0wLjI4LDEuNDgzLTAuOTI1YzAtMC41NDctMC4yOTItMC43OS0xLjg5Ny0xLjE1NWMtMS41NjgtMC4zNjQtMi42NjItMC44NjMtMi42NjItMi41NDINCgkJCWMwLTEuNTQ0LDEuMzYtMi40OTMsMy4wMzktMi40OTNjMS4zMjUsMCwyLjI5OSwwLjQ5OSwzLjEwMiwxLjMzN0wyMDUuNiwzLjY2NGMtMC41NDgtMC41Ni0xLjE1NC0wLjg3NS0xLjkzNC0wLjg3NQ0KCQkJYy0wLjkxMiwwLTEuMjE3LDAuMzg5LTEuMjE3LDAuNzY2YzAsMC41MzUsMC4zNjUsMC43MDYsMS43MTUsMS4wMjFjMS4zNTIsMC4zMTYsMi44NDYsMC43NzgsMi44NDYsMi42MjcNCgkJCUMyMDcuMDEsOC44MiwyMDUuOTg4LDkuOTE1LDIwMy42NjYsOS45MTV6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0yMDguODA5LDkuNzgxVjEuMjY5aDUuOTF2MS42NjZoLTQuMjA3djEuNDcxaDIuNDQzdjEuNjU0aC0yLjQ0M3YyLjA1Nmg0LjM5MXYxLjY2NkwyMDguODA5LDkuNzgxDQoJCQlMMjA4LjgwOSw5Ljc4MXoiLz4NCgk8L2c+DQoJPGc+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik00LjY1Myw5Ljc4MUwzLjI2Myw2LjkxSDIuMzEydjIuODcxSDBWMS4yNjZoMy44MDhjMC40OTQsMCwwLjk0NywwLjA1MSwxLjM1NiwwLjE1Mg0KCQkJQzUuNTc0LDEuNTIsNS45MjUsMS42OCw2LjIxNywxLjg5N2MwLjI5MiwwLjIyLDAuNTE4LDAuNTA1LDAuNjc2LDAuODU3YzAuMTU3LDAuMzU0LDAuMjM2LDAuNzgsMC4yMzYsMS4yODUNCgkJCWMwLDAuNjQxLTAuMTM4LDEuMTYzLTAuNDE0LDEuNTY4QzYuNDM4LDYuMDE0LDYuMDY0LDYuMzIsNS41OTIsNi41MzFsMS43MDcsMy4yNUg0LjY1M3ogTTQuNTQzLDMuNDQyDQoJCQljLTAuMTU5LTAuMTctMC40MjgtMC4yNTYtMC44MDEtMC4yNTZoLTEuNDN2MS44NjJoMS4zOTRjMC4zOTIsMCwwLjY2OC0wLjA4LDAuODMxLTAuMjQzQzQuNyw0LjY0NSw0Ljc4Miw0LjQwOCw0Ljc4Miw0LjEwMQ0KCQkJQzQuNzgyLDMuODMzLDQuNzAyLDMuNjEzLDQuNTQzLDMuNDQyeiIvPg0KCQk8cGF0aCBmaWxsPSIjRkZGRkZGIiBkPSJNOC43MSw5Ljc4MVYxLjI2Nmg2LjUyMXYxLjk4M2gtNC4xODR2MS4xNDRoMi41MThWNi4zNGgtMi41MTh2MS40NTloNC4yNjl2MS45ODJIOC43MXoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTIzLjg5MSw3LjVjLTAuMTkzLDAuNTQ5LTAuNDg0LDAuOTg4LTAuODc0LDEuMzI2Yy0wLjM5MSwwLjMzOC0wLjg3LDAuNTc4LTEuNDQzLDAuNzMNCgkJCWMtMC41NywwLjE0Ny0xLjIzNCwwLjIyNS0xLjk4OCwwLjIyNWgtMi43NjFWMS4yNjZoMi45NzljMC42NjUsMCwxLjI3MSwwLjA2OCwxLjgxMiwwLjIwN2MwLjU0MiwwLjEzOCwxLjAwNCwwLjM3MSwxLjM4MSwwLjY5OQ0KCQkJczAuNjY5LDAuNzYsMC44NzYsMS4yOTVjMC4yMDYsMC41MzYsMC4zMSwxLjIwNSwwLjMxLDIuMDA4QzI0LjE4NCw2LjI3NSwyNC4wODYsNi45NTEsMjMuODkxLDcuNXogTTIxLjY2LDQuNTAzDQoJCQljLTAuMDctMC4yODUtMC4xODItMC41MTYtMC4zMzQtMC42OTRjLTAuMTU1LTAuMTc5LTAuMzU4LTAuMzEtMC42MDgtMC4zOTZjLTAuMjUyLTAuMDg1LTAuNTYtMC4xMjgtMC45MjQtMC4xMjhoLTAuNTg2djQuNDc5DQoJCQloMC41MTJjMC4zNjUsMCwwLjY3OC0wLjAzOSwwLjkzNy0wLjExNWMwLjI1OS0wLjA3NiwwLjQ3Mi0wLjIwMywwLjYzOC0wLjM3N2MwLjE2Ny0wLjE3NCwwLjI4Ni0wLjQwNCwwLjM1OS0wLjY5Mw0KCQkJYzAuMDczLTAuMjg3LDAuMTA5LTAuNjQzLDAuMTA5LTEuMDY0QzIxLjc2Myw1LjEyMiwyMS43MjksNC43ODUsMjEuNjYsNC41MDN6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik0zNC42ODEsOS43ODFWNi40MjRIMzIuMDN2My4zNTdoLTIuNDA5VjEuMjY2aDIuNDA5djMuMTAzaDIuNjUxVjEuMjY2aDIuNDA5djguNTE2TDM0LjY4MSw5Ljc4MQ0KCQkJTDM0LjY4MSw5Ljc4MXoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTQ0LjMxNiw5Ljc4MWwtMC40NjItMS40OThINDEuM2wtMC40NjMsMS40OThoLTIuNTNsMy4wOS04LjUxNmgyLjM4NWwzLjA4OSw4LjUxNkg0NC4zMTZ6IE00My4xMjUsNS44NTINCgkJCWMtMC4wNzMtMC4yNzYtMC4xMzktMC41MTUtMC4xOTUtMC43MTljLTAuMDU3LTAuMjAxLTAuMTA3LTAuMzg1LTAuMTUyLTAuNTQ3Yy0wLjA0NS0wLjE2Mi0wLjA4My0wLjMxMS0wLjExNS0wLjQ0Mw0KCQkJYy0wLjAzMi0wLjEzMi0wLjA2MS0wLjI3NC0wLjA4Ni0wLjQyYy0wLjAyMywwLjE0Ni0wLjA1MiwwLjI4OC0wLjA4NSwwLjQyNWMtMC4wMzIsMC4xMzgtMC4wNywwLjI4OS0wLjExNCwwLjQ1MQ0KCQkJYy0wLjA0NSwwLjE2MS0wLjA5NiwwLjM0NC0wLjE1MywwLjU0N2MtMC4wNTYsMC4yMDMtMC4xMjEsMC40MzctMC4xOTQsMC43MDRMNDEuODczLDYuNDFoMS40MUw0My4xMjUsNS44NTJ6Ii8+DQoJCTxwYXRoIGZpbGw9IiNGRkZGRkYiIGQ9Ik01MS4zMjQsMy4zMjJ2Ni40NTloLTIuMzZWMy4zMjJoLTIuMzg1VjEuMjY2aDcuMTI5djIuMDU3aC0yLjM4NFYzLjMyMnoiLz4NCgkJPHBhdGggZmlsbD0iI0ZGRkZGRiIgZD0iTTU5LjAwMiwyLjIyOWMtMC4wODEsMC4xOTQtMC4xOTIsMC4zNjMtMC4zMzcsMC41MDZjLTAuMTQ0LDAuMTQ1LTAuMzExLDAuMjU1LTAuNTA2LDAuMzM3DQoJCQljLTAuMTk1LDAuMDgxLTAuNDA1LDAuMTIyLTAuNjMyLDAuMTIycy0wLjQzOC0wLjA0MS0wLjYzMy0wLjEyMmMtMC4xOTUtMC4wODEtMC4zNjQtMC4xOTEtMC41MDctMC4zMzcNCgkJCWMtMC4xNDMtMC4xNDQtMC4yNTUtMC4zMTItMC4zMzUtMC41MDZjLTAuMDgyLTAuMTk0LTAuMTIzLTAuNDA1LTAuMTIzLTAuNjMyYzAtMC4yMjgsMC4wNDEtMC40MzksMC4xMjMtMC42MzMNCgkJCWMwLjA3OS0wLjE5NCwwLjE5MS0wLjM2MywwLjMzNS0wLjUwNmMwLjE0NC0wLjE0NCwwLjMxMi0wLjI1NiwwLjUwNy0wLjMzNkM1Ny4wODksMC4wNDEsNTcuMywwLDU3LjUyNywwczAuNDM4LDAuMDQxLDAuNjMyLDAuMTIzDQoJCQljMC4xOTQsMC4wNzksMC4zNjMsMC4xOTEsMC41MDYsMC4zMzZjMC4xNDQsMC4xNDMsMC4yNTUsMC4zMTIsMC4zMzcsMC41MDZjMC4wODEsMC4xOTIsMC4xMjIsMC40MDUsMC4xMjIsMC42MzMNCgkJCVM1OS4wODMsMi4wMzUsNTkuMDAyLDIuMjI5eiBNNTguNywxLjA3M2MtMC4wNjUtMC4xNTgtMC4xNTMtMC4yOTMtMC4yNjUtMC40MDRjLTAuMTEyLTAuMTEyLTAuMjQ3LTAuMTk5LTAuNDAzLTAuMjYzDQoJCQljLTAuMTU2LTAuMDYyLTAuMzIzLTAuMDkzLTAuNTA0LTAuMDkzYy0wLjE4NCwwLTAuMzUyLDAuMDMxLTAuNTA3LDAuMDkzYy0wLjE1NiwwLjA2Mi0wLjI4OSwwLjE0OC0wLjQsMC4yNjMNCgkJCWMtMC4xMTIsMC4xMTEtMC4yLDAuMjQ2LTAuMjY2LDAuNDA0Yy0wLjA2NCwwLjE1Ny0wLjA5NywwLjMzMS0wLjA5NywwLjUyNGMwLDAuMTkxLDAuMDMyLDAuMzY2LDAuMDk3LDAuNTIyDQoJCQljMC4wNjUsMC4xNTksMC4xNTMsMC4yOTQsMC4yNjYsMC40MDVjMC4xMTEsMC4xMTEsMC4yNDUsMC4xOTksMC40LDAuMjYyYzAuMTU1LDAuMDYyLDAuMzIzLDAuMDk0LDAuNTA3LDAuMDk0DQoJCQljMC4xODEsMCwwLjM0OC0wLjAzMSwwLjUwNC0wLjA5NGMwLjE1Ny0wLjA2MiwwLjI5MS0wLjE0OCwwLjQwMy0wLjI2MmMwLjExMS0wLjExMSwwLjE5OS0wLjI0NiwwLjI2NS0wLjQwNQ0KCQkJYzAuMDY0LTAuMTU2LDAuMDk3LTAuMzMxLDAuMDk3LTAuNTIyQzU4Ljc5NiwxLjQwNCw1OC43NjQsMS4yMyw1OC43LDEuMDczeiBNNTcuODEsMi40MjZsLTAuMjgyLTAuNTg0aC0wLjE5NHYwLjU4NEg1Ni44NlYwLjY4OQ0KCQkJaDAuNzc4YzAuMjAzLDAsMC4zNjYsMC4wNDQsMC40OSwwLjEyOWMwLjEyNSwwLjA4NiwwLjE4NywwLjIzMiwwLjE4NywwLjQzOGMwLDAuMTMzLTAuMDI5LDAuMjM4LTAuMDg3LDAuMzINCgkJCWMtMC4wNTcsMC4wODEtMC4xMzIsMC4xNDUtMC4yMjcsMC4xODhsMC4zNDQsMC42NTlMNTcuODEsMi40MjZMNTcuODEsMi40MjZ6IE01Ny43ODgsMS4xMzVjLTAuMDMzLTAuMDM0LTAuMDg4LTAuMDUxLTAuMTY0LTAuMDUxDQoJCQloLTAuMjkxVjEuNDZoMC4yODNjMC4wNzksMCwwLjEzNi0wLjAxNywwLjE2OS0wLjA0OWMwLjAzNS0wLjAzMiwwLjA1MS0wLjA3OSwwLjA1MS0wLjE0M0M1Ny44MzYsMS4yMTUsNTcuODIxLDEuMTcxLDU3Ljc4OCwxLjEzNQ0KCQkJeiIvPg0KCTwvZz4NCjwvZz4NCjwvc3ZnPg0K" alt="Red Hat&reg; OpenShift Enterprise">
          </div><!--/#brand-->
          {{ if .Error }}
          <div class="alert alert-danger">
            <span class="pficon-layered">
              <span class="pficon pficon-error-octagon"></span>
              <span class="pficon pficon-error-exclamation"></span>
            </span>
            {{ .Error }}
          </div>
          {{ end }}
        </div><!--/.col-*-->
        <div class="col-sm-7 col-md-6 col-lg-5 login">
          <form class="form-horizontal" role="form" action="{{ .Action }}" method="POST">
            <input type="hidden" name="{{ .Names.Then }}" value="{{ .Values.Then }}">
            <input type="hidden" name="{{ .Names.CSRF }}" value="{{ .Values.CSRF }}">
            <div class="form-group">
              <label for="inputUsername" class="col-sm-2 col-md-2 control-label">Username</label>
              <div class="col-sm-10 col-md-10">
                <input type="text" class="form-control" id="inputUsername" placeholder="" tabindex="1" autofocus="autofocus" type="text" name="{{ .Names.Username }}" value="{{ .Values.Username }}">
              </div>
            </div>
            <div class="form-group">
              <label for="inputPassword" class="col-sm-2 col-md-2 control-label">Password</label>
              <div class="col-sm-10 col-md-10">
                <input type="password" class="form-control" id="inputPassword" placeholder="" tabindex="2" type="password" name="{{ .Names.Password }}" value="">
              </div>
            </div>
            <div class="form-group">
              <div class="col-xs-8 col-sm-offset-2 col-sm-6 col-md-offset-2 col-md-6">
              <!--
                <div class="checkbox">
                  <label>
                    <input type="checkbox" tabindex="3"> Remember Username
                  </label>
                </div>
                <span class="help-block"> Forgot <a href="#" tabindex="5">Username</a> or <a href="#" tabindex="6">Password</a>?</span>
              -->
              </div>
              <div class="col-xs-4 col-sm-4 col-md-4 submit">
                <button type="submit" class="btn btn-primary btn-lg" tabindex="4">Log In</button>
              </div>
            </div>
          </form>
        </div><!--/.col-*-->
        <div class="col-sm-5 col-md-6 col-lg-7 details">
          </p>
          <p><strong>Welcome to Red Hat&reg; OpenShift Enterprise.</strong>
        </div><!--/.col-*-->
      </div><!--/.row-->
    </div><!--/.container-->
  </body>
</html>
`
