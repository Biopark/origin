package node

import (
	"fmt"
	"net"
	"sync"
	"syscall"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/util/sets"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	networkapi "github.com/openshift/origin/pkg/network/apis/network"
	"github.com/openshift/origin/pkg/network/common"
	networkclient "github.com/openshift/origin/pkg/network/generated/internalclientset"

	"github.com/vishvananda/netlink"
)

type nodeEgress struct {
	nodeIP string

	// requestedIPs are the EgressIPs listed on the node's HostSubnet
	requestedIPs sets.String
	// assignedIPs are the IPs actually in use on the node
	assignedIPs sets.String
}

type namespaceEgress struct {
	vnid uint32

	// requestedIP is the egress IP it wants (NetNamespace.EgressIPs[0])
	requestedIP string
	// assignedIP is an egress IP actually in use on nodeIP
	assignedIP string
	nodeIP     string
}

type egressIPWatcher struct {
	sync.Mutex

	localIP string
	oc      *ovsController

	networkClient networkclient.Interface
	iptables      *NodeIPTables

	// from HostSubnets
	nodesByNodeIP   map[string]*nodeEgress
	nodesByEgressIP map[string]*nodeEgress

	// From NetNamespaces
	namespacesByVNID     map[uint32]*namespaceEgress
	namespacesByEgressIP map[string]*namespaceEgress

	localEgressLink netlink.Link
	localEgressNet  *net.IPNet

	testModeChan chan string
}

func newEgressIPWatcher(localIP string, oc *ovsController) *egressIPWatcher {
	return &egressIPWatcher{
		localIP: localIP,
		oc:      oc,

		nodesByNodeIP:   make(map[string]*nodeEgress),
		nodesByEgressIP: make(map[string]*nodeEgress),

		namespacesByVNID:     make(map[uint32]*namespaceEgress),
		namespacesByEgressIP: make(map[string]*namespaceEgress),
	}
}

func (eip *egressIPWatcher) Start(networkClient networkclient.Interface, iptables *NodeIPTables) error {
	var err error
	if eip.localEgressLink, eip.localEgressNet, err = GetLinkDetails(eip.localIP); err != nil {
		// Not expected, should already be caught by node.New()
		return nil
	}

	eip.iptables = iptables
	eip.networkClient = networkClient

	go utilwait.Forever(eip.watchHostSubnets, 0)
	go utilwait.Forever(eip.watchNetNamespaces, 0)
	return nil
}

func ipToHex(ip string) string {
	bytes := net.ParseIP(ip)
	if bytes == nil {
		return "invalid IP: shouldn't happen"
	}
	bytes = bytes.To4()
	return fmt.Sprintf("0x%02x%02x%02x%02x", bytes[0], bytes[1], bytes[2], bytes[3])
}

func (eip *egressIPWatcher) watchHostSubnets() {
	common.RunEventQueue(eip.networkClient.Network().RESTClient(), common.HostSubnets, func(delta cache.Delta) error {
		hs := delta.Object.(*networkapi.HostSubnet)

		var egressIPs []string
		if delta.Type != cache.Deleted {
			egressIPs = hs.EgressIPs
		}

		eip.updateNodeEgress(hs.HostIP, egressIPs)
		return nil
	})
}

func (eip *egressIPWatcher) updateNodeEgress(nodeIP string, nodeEgressIPs []string) {
	eip.Lock()
	defer eip.Unlock()

	node := eip.nodesByNodeIP[nodeIP]
	if node == nil {
		if len(nodeEgressIPs) == 0 {
			return
		}
		node = &nodeEgress{
			nodeIP:       nodeIP,
			requestedIPs: sets.NewString(),
			assignedIPs:  sets.NewString(),
		}
		eip.nodesByNodeIP[nodeIP] = node
	} else if len(nodeEgressIPs) == 0 {
		delete(eip.nodesByNodeIP, nodeIP)
	}
	oldRequestedIPs := node.requestedIPs
	node.requestedIPs = sets.NewString(nodeEgressIPs...)

	// Process new EgressIPs
	for _, ip := range node.requestedIPs.Difference(oldRequestedIPs).UnsortedList() {
		if oldNode := eip.nodesByEgressIP[ip]; oldNode != nil {
			glog.Errorf("Multiple nodes claiming EgressIP %q (nodes %q, %q)", ip, node.nodeIP, oldNode.nodeIP)
			continue
		}

		eip.nodesByEgressIP[ip] = node
		eip.maybeAddEgressIP(ip)
	}

	// Process removed EgressIPs
	for _, ip := range oldRequestedIPs.Difference(node.requestedIPs).UnsortedList() {
		if oldNode := eip.nodesByEgressIP[ip]; oldNode != node {
			// User removed a duplicate EgressIP
			continue
		}

		eip.deleteEgressIP(ip)
		delete(eip.nodesByEgressIP, ip)
	}
}

func (eip *egressIPWatcher) maybeAddEgressIP(egressIP string) {
	node := eip.nodesByEgressIP[egressIP]
	ns := eip.namespacesByEgressIP[egressIP]
	if ns == nil {
		return
	}

	hex := ipToHex(egressIP)
	nodeIP := ""

	if node != nil && !node.assignedIPs.Has(egressIP) {
		node.assignedIPs.Insert(egressIP)
		nodeIP = node.nodeIP
		if node.nodeIP == eip.localIP {
			if err := eip.assignEgressIP(egressIP, hex); err != nil {
				glog.Errorf("Error assigning Egress IP %q: %v", egressIP, err)
				nodeIP = ""
			}
		}
	}

	if ns.assignedIP != egressIP || ns.nodeIP != nodeIP {
		ns.assignedIP = egressIP
		ns.nodeIP = nodeIP

		err := eip.oc.UpdateNamespaceEgressRules(ns.vnid, ns.nodeIP, hex)
		if err != nil {
			glog.Errorf("Error updating Namespace egress rules: %v", err)
		}
	}
}

func (eip *egressIPWatcher) deleteEgressIP(egressIP string) {
	node := eip.nodesByEgressIP[egressIP]
	ns := eip.namespacesByEgressIP[egressIP]
	if node == nil || ns == nil {
		return
	}

	hex := ipToHex(egressIP)
	if node.nodeIP == eip.localIP {
		if err := eip.releaseEgressIP(egressIP, hex); err != nil {
			glog.Errorf("Error releasing Egress IP %q: %v", egressIP, err)
		}
	}

	if ns.assignedIP == egressIP {
		ns.assignedIP = ""
		ns.nodeIP = ""
	}

	var err error
	if ns.requestedIP == "" {
		// Namespace no longer wants EgressIP
		err = eip.oc.UpdateNamespaceEgressRules(ns.vnid, "", "")
	} else {
		// Namespace still wants EgressIP but no node provides it
		err = eip.oc.UpdateNamespaceEgressRules(ns.vnid, "", hex)
	}
	if err != nil {
		glog.Errorf("Error updating Namespace egress rules: %v", err)
	}
}

func (eip *egressIPWatcher) watchNetNamespaces() {
	common.RunEventQueue(eip.networkClient.Network().RESTClient(), common.NetNamespaces, func(delta cache.Delta) error {
		netns := delta.Object.(*networkapi.NetNamespace)

		if delta.Type != cache.Deleted && len(netns.EgressIPs) != 0 {
			if len(netns.EgressIPs) > 1 {
				glog.Warningf("Ignoring extra EgressIPs (%v) in NetNamespace %q", netns.EgressIPs[1:], netns.Name)
			}
			eip.updateNamespaceEgress(netns.NetID, netns.EgressIPs[0])
		} else {
			eip.deleteNamespaceEgress(netns.NetID)
		}
		return nil
	})
}

func (eip *egressIPWatcher) updateNamespaceEgress(vnid uint32, egressIP string) {
	eip.Lock()
	defer eip.Unlock()

	ns := eip.namespacesByVNID[vnid]
	if ns == nil {
		ns = &namespaceEgress{vnid: vnid}
		eip.namespacesByVNID[vnid] = ns
	}
	if ns.requestedIP == egressIP {
		return
	}
	if oldNS := eip.namespacesByEgressIP[egressIP]; oldNS != nil {
		glog.Errorf("Multiple NetNamespaces claiming EgressIP %q (NetIDs %d, %d)", egressIP, ns.vnid, oldNS.vnid)
		return
	}

	if ns.assignedIP != "" {
		eip.deleteEgressIP(egressIP)
		delete(eip.namespacesByEgressIP, egressIP)
		ns.assignedIP = ""
		ns.nodeIP = ""
	}
	ns.requestedIP = egressIP
	eip.namespacesByEgressIP[egressIP] = ns
	eip.maybeAddEgressIP(egressIP)
}

func (eip *egressIPWatcher) deleteNamespaceEgress(vnid uint32) {
	eip.Lock()
	defer eip.Unlock()

	ns := eip.namespacesByVNID[vnid]
	if ns == nil {
		return
	}

	if ns.assignedIP != "" {
		ns.requestedIP = ""
		eip.deleteEgressIP(ns.assignedIP)
		delete(eip.namespacesByEgressIP, ns.assignedIP)
	}
	delete(eip.namespacesByVNID, vnid)
}

func (eip *egressIPWatcher) assignEgressIP(egressIP, egressHex string) error {
	if egressIP == eip.localIP {
		return fmt.Errorf("desired egress IP %q is the node IP", egressIP)
	}

	if eip.testModeChan != nil {
		eip.testModeChan <- fmt.Sprintf("claim %s", egressIP)
		return nil
	}

	localEgressIPMaskLen, _ := eip.localEgressNet.Mask.Size()
	egressIPNet := fmt.Sprintf("%s/%d", egressIP, localEgressIPMaskLen)
	addr, err := netlink.ParseAddr(egressIPNet)
	if err != nil {
		return fmt.Errorf("could not parse egress IP %q: %v", egressIPNet, err)
	}
	if !eip.localEgressNet.Contains(addr.IP) {
		return fmt.Errorf("egress IP %q is not in local network %s of interface %s", egressIP, eip.localEgressNet.String(), eip.localEgressLink.Attrs().Name)
	}
	err = netlink.AddrAdd(eip.localEgressLink, addr)
	if err != nil {
		if err == syscall.EEXIST {
			glog.V(2).Infof("Egress IP %q already exists on %s", egressIPNet, eip.localEgressLink.Attrs().Name)
		} else {
			return fmt.Errorf("could not add egress IP %q to %s: %v", egressIPNet, eip.localEgressLink.Attrs().Name, err)
		}
	}

	if err := eip.iptables.AddEgressIPRules(egressIP, egressHex); err != nil {
		return fmt.Errorf("could not add egress IP iptables rule: %v", err)
	}

	return nil
}

func (eip *egressIPWatcher) releaseEgressIP(egressIP, egressHex string) error {
	if egressIP == eip.localIP {
		return nil
	}

	if eip.testModeChan != nil {
		eip.testModeChan <- fmt.Sprintf("release %s", egressIP)
		return nil
	}

	localEgressIPMaskLen, _ := eip.localEgressNet.Mask.Size()
	egressIPNet := fmt.Sprintf("%s/%d", egressIP, localEgressIPMaskLen)
	addr, err := netlink.ParseAddr(egressIPNet)
	if err != nil {
		return fmt.Errorf("could not parse egress IP %q: %v", egressIPNet, err)
	}
	err = netlink.AddrDel(eip.localEgressLink, addr)
	if err != nil {
		if err == syscall.EADDRNOTAVAIL {
			glog.V(2).Infof("Could not delete egress IP %q from %s: no such address", egressIPNet, eip.localEgressLink.Attrs().Name)
		} else {
			return fmt.Errorf("could not delete egress IP %q from %s: %v", egressIPNet, eip.localEgressLink.Attrs().Name, err)
		}
	}

	if err := eip.iptables.DeleteEgressIPRules(egressIP, egressHex); err != nil {
		return fmt.Errorf("could not delete egress IP iptables rule: %v", err)
	}

	return nil
}
