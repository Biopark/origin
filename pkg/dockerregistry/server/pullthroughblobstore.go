package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
)

// pullthroughBlobStore wraps a distribution.BlobStore and allows remote repositories to serve blobs from remote
// repositories.
type pullthroughBlobStore struct {
	distribution.BlobStore

	repo *repository
}

var _ distribution.BlobStore = &pullthroughBlobStore{}

// Stat makes a local check for the blob, then falls through to the other servers referenced by
// the image stream and looks for those that have the layer.
func (r *pullthroughBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	// check the local store for the blob
	desc, err := r.BlobStore.Stat(ctx, dgst)
	switch {
	case err == distribution.ErrBlobUnknown:
		// continue on to the code below and look up the blob in a remote store since it is not in
		// the local store
	case err != nil:
		context.GetLogger(ctx).Errorf("Failed to find blob %q: %#v", dgst.String(), err)
		fallthrough
	default:
		return desc, err
	}

	remoteGetter, found := RemoteBlobGetterFrom(r.repo.ctx)
	if !found {
		context.GetLogger(ctx).Errorf("pullthroughBlobStore.Stat: failed to retrieve remote getter from context")
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	return remoteGetter.Stat(ctx, dgst)
}

// ServeBlob attempts to serve the requested digest onto w, using a remote proxy store if necessary.
func (r *pullthroughBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, req *http.Request, dgst digest.Digest) error {
	// This call should be done without BlobGetterService in the context.
	err := r.BlobStore.ServeBlob(ctx, w, req, dgst)
	switch {
	case err == distribution.ErrBlobUnknown:
		// continue on to the code below and look up the blob in a remote store since it is not in
		// the local store
	case err != nil:
		context.GetLogger(ctx).Errorf("Failed to find blob %q: %#v", dgst.String(), err)
		fallthrough
	default:
		return err
	}

	remoteGetter, found := RemoteBlobGetterFrom(r.repo.ctx)
	if !found {
		context.GetLogger(ctx).Errorf("pullthroughBlobStore.ServeBlob: failed to retrieve remote getter from context")
		return distribution.ErrBlobUnknown
	}

	desc, err := remoteGetter.Stat(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to stat digest %q: %v", dgst.String(), err)
		return err
	}

	remoteReader, err := remoteGetter.Open(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("failure to open remote store for digest %q: %v", dgst.String(), err)
		return err
	}
	defer remoteReader.Close()

	context.GetLogger(ctx).Infof("serving blob %s of type %s %d bytes long", dgst.String(), desc.MediaType, desc.Size)
	contentHandled, err := serveRemoteContent(w, req, desc, remoteReader)
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to serve blob %s: %v", dgst.String(), err)
		return err
	}
	if contentHandled {
		context.GetLogger(ctx).Debugf("the blob %s has been successfully served", dgst.String())
		return nil
	}

	context.GetLogger(ctx).Debugf("the stream isn't seek-able, falling back to io.copy")

	w.Header().Set("Content-Length", fmt.Sprintf("%d", desc.Size))

	_, err = io.CopyN(w, remoteReader, desc.Size)
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to serve blob %s: %v", dgst.String(), err)
		return err
	}

	context.GetLogger(ctx).Debugf("the blob %s has been successfully served", dgst.String())
	return nil
}

// Get attempts to fetch the requested blob by digest using a remote proxy store if necessary.
func (r *pullthroughBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	data, originalErr := r.BlobStore.Get(ctx, dgst)
	if originalErr == nil {
		return data, nil
	}

	remoteGetter, found := RemoteBlobGetterFrom(r.repo.ctx)
	if !found {
		context.GetLogger(ctx).Errorf("pullthroughBlobStore.Get: failed to retrieve remote getter from context")
		return nil, originalErr
	}

	return remoteGetter.Get(ctx, dgst)
}

// setResponseHeaders sets the appropriate content serving headers
func setResponseHeaders(w http.ResponseWriter, length int64, mediaType string, digest digest.Digest) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest.String())
	w.Header().Set("Etag", digest.String())
}

// serveRemoteContent tries to use http.ServeContent for remote content.
func serveRemoteContent(rw http.ResponseWriter, req *http.Request, desc distribution.Descriptor, remoteReader io.ReadSeeker) (bool, error) {
	// Set the appropriate content serving headers.
	setResponseHeaders(rw, desc.Size, desc.MediaType, desc.Digest)

	// Fallback to Copy if request wasn't given.
	if req == nil {
		return false, nil
	}

	// Check whether remoteReader is seekable. The remoteReader' Seek method must work: ServeContent uses
	// a seek to the end of the content to determine its size.
	if _, err := remoteReader.Seek(0, os.SEEK_END); err != nil {
		// The remoteReader isn't seekable. It means that the remote response under the hood of remoteReader
		// doesn't contain any Content-Range or Content-Length headers. In this case we need to rollback to
		// simple Copy.
		return false, nil
	}

	// Move pointer back to begin.
	if _, err := remoteReader.Seek(0, os.SEEK_SET); err != nil {
		return false, err
	}

	http.ServeContent(rw, req, desc.Digest.String(), time.Time{}, remoteReader)

	return true, nil
}
