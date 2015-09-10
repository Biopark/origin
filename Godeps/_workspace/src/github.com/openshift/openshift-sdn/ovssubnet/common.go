package ovssubnet

import (
	"errors"
	"fmt"
	log "github.com/golang/glog"
	"net"
	"time"

	"github.com/openshift/openshift-sdn/ovssubnet/api"
	"github.com/openshift/openshift-sdn/ovssubnet/controller/kube"
	"github.com/openshift/openshift-sdn/ovssubnet/controller/lbr"
	"github.com/openshift/openshift-sdn/ovssubnet/controller/multitenant"
	"github.com/openshift/openshift-sdn/pkg/netutils"
)

const (
	// Maximum VXLAN Network Identifier as per RFC#7348
	MaxVNID = ((1 << 24) - 1)
)

type OvsController struct {
	subnetRegistry  api.SubnetRegistry
	localIP         string
	localSubnet     *api.Subnet
	hostName        string
	subnetAllocator *netutils.SubnetAllocator
	sig             chan struct{}
	ready           chan struct{}
	flowController  FlowController
	VNIDMap         map[string]uint
	netIDManager    *netutils.NetIDAllocator
	AdminNamespaces []string
}

type FlowController interface {
	Setup(localSubnetIP, globalSubnetIP, serviceSubnetIP string, mtu uint) error
	AddOFRules(nodeIP, localSubnetIP, localIP string) error
	DelOFRules(nodeIP, localIP string) error
	AddServiceOFRules(netID uint, IP string, protocol api.ServiceProtocol, port uint) error
	DelServiceOFRules(netID uint, IP string, protocol api.ServiceProtocol, port uint) error
}

func NewKubeController(sub api.SubnetRegistry, hostname string, selfIP string, ready chan struct{}) (*OvsController, error) {
	kubeController, err := NewController(sub, hostname, selfIP, ready)
	if err == nil {
		kubeController.flowController = kube.NewFlowController()
	}
	return kubeController, err
}

func NewMultitenantController(sub api.SubnetRegistry, hostname string, selfIP string, ready chan struct{}) (*OvsController, error) {
	mtController, err := NewController(sub, hostname, selfIP, ready)
	if err == nil {
		mtController.flowController = multitenant.NewFlowController()
	}
	return mtController, err
}

func NewDefaultController(sub api.SubnetRegistry, hostname string, selfIP string, ready chan struct{}) (*OvsController, error) {
	defaultController, err := NewController(sub, hostname, selfIP, ready)
	if err == nil {
		defaultController.flowController = lbr.NewFlowController()
	}
	return defaultController, err
}

func NewController(sub api.SubnetRegistry, hostname string, selfIP string, ready chan struct{}) (*OvsController, error) {
	if selfIP == "" {
		var err error
		selfIP, err = GetNodeIP(hostname)
		if err != nil {
			return nil, err
		}
	}
	log.Infof("Self IP: %s.", selfIP)
	return &OvsController{
		subnetRegistry:  sub,
		localIP:         selfIP,
		hostName:        hostname,
		localSubnet:     nil,
		subnetAllocator: nil,
		VNIDMap:         make(map[string]uint),
		sig:             make(chan struct{}),
		ready:           ready,
		AdminNamespaces: make([]string, 0),
	}, nil
}

func (oc *OvsController) StartMaster(sync bool, containerNetwork string, containerSubnetLength uint, serviceNetwork string) error {
	// wait a minute for etcd to come alive
	status := oc.subnetRegistry.CheckEtcdIsAlive(60)
	if !status {
		log.Errorf("Etcd not running?")
		return errors.New("Etcd not reachable. Sync cluster check failed.")
	}
	// initialize the node key
	if sync {
		err := oc.subnetRegistry.InitNodes()
		if err != nil {
			log.Infof("Node path already initialized.")
		}
	}

	// initialize the subnet key?
	oc.subnetRegistry.InitSubnets()
	subrange := make([]string, 0)
	subnets, err := oc.subnetRegistry.GetSubnets()
	if err != nil {
		log.Errorf("Error in initializing/fetching subnets: %v", err)
		return err
	}
	for _, sub := range subnets {
		subrange = append(subrange, sub.SubnetIP)
	}

	err = oc.subnetRegistry.WriteNetworkConfig(containerNetwork, containerSubnetLength, serviceNetwork)
	if err != nil {
		return err
	}

	oc.subnetAllocator, err = netutils.NewSubnetAllocator(containerNetwork, containerSubnetLength, subrange)
	if err != nil {
		return err
	}
	err = oc.ServeExistingNodes()
	if err != nil {
		log.Warningf("Error initializing existing nodes: %v", err)
		// no worry, we can still keep watching it.
	}
	if _, is_mt := oc.flowController.(*multitenant.FlowController); is_mt {
		nets, err := oc.subnetRegistry.GetNetNamespaces()
		if err != nil {
			return err
		}
		inUse := make([]uint, 0)
		for _, net := range nets {
			inUse = append(inUse, net.NetID)
			oc.VNIDMap[net.Name] = net.NetID
		}
		// VNID: 0 reserved for default namespace and can reach any network in the cluster
		// VNID: 1 to 9 are internally reserved for any special cases in the future
		oc.netIDManager, err = netutils.NewNetIDAllocator(10, MaxVNID, inUse)
		if err != nil {
			return err
		}

		// Handle existing namespaces without VNID
		existingNamespaces, err := oc.subnetRegistry.GetNamespaces()
		if err != nil {
			return err
		}
		for _, nsName := range existingNamespaces {
			// Skip admin namespaces, they will have VNID: 0
			if oc.isAdminNamespace(nsName) {
				// Revoke VNID if already exists
				if _, ok := oc.VNIDMap[nsName]; ok {
					err := oc.revokeVNID(nsName)
					if err != nil {
						return err
					}
				}
				continue
			}
			// Skip if VNID already exists for the namespace
			if _, ok := oc.VNIDMap[nsName]; ok {
				continue
			}
			err := oc.assignVNID(nsName)
			if err != nil {
				return err
			}
		}
		go oc.watchNetworks()
	}
	go oc.watchNodes()
	return nil
}

func (oc *OvsController) isAdminNamespace(nsName string) bool {
	for _, name := range oc.AdminNamespaces {
		if name == nsName {
			return true
		}
	}
	return false
}

func (oc *OvsController) assignVNID(namespaceName string) error {
	_, err := oc.subnetRegistry.GetNetNamespace(namespaceName)
	if err != nil {
		netid, err := oc.netIDManager.GetNetID()
		if err != nil {
			return err
		}
		err = oc.subnetRegistry.WriteNetNamespace(namespaceName, netid)
		if err != nil {
			e := oc.netIDManager.ReleaseNetID(netid)
			if e != nil {
				log.Error("Error while releasing Net ID: %v", e)
			}
			return err
		}
		oc.VNIDMap[namespaceName] = netid
	}
	return nil
}

func (oc *OvsController) revokeVNID(namespaceName string) error {
	err := oc.subnetRegistry.DeleteNetNamespace(namespaceName)
	if err != nil {
		return err
	}
	netid, ok := oc.VNIDMap[namespaceName]
	if !ok {
		return fmt.Errorf("Error while fetching Net ID for namespace: %s", namespaceName)
	}
	err = oc.netIDManager.ReleaseNetID(netid)
	if err != nil {
		return fmt.Errorf("Error while releasing Net ID: %v", err)
	}
	delete(oc.VNIDMap, namespaceName)
	return nil
}

func (oc *OvsController) watchNetworks() {
	nsevent := make(chan *api.NamespaceEvent)
	stop := make(chan bool)
	go oc.subnetRegistry.WatchNamespaces(nsevent, stop)
	for {
		select {
		case ev := <-nsevent:
			switch ev.Type {
			case api.Added:
				err := oc.assignVNID(ev.Name)
				if err != nil {
					log.Error("Error assigning Net ID: %v", err)
					continue
				}
			case api.Deleted:
				err := oc.revokeVNID(ev.Name)
				if err != nil {
					log.Error("Error revoking Net ID: %v", err)
					continue
				}
			}
		case <-oc.sig:
			log.Error("Signal received. Stopping watching of nodes.")
			stop <- true
			return
		}
	}
}

func (oc *OvsController) ServeExistingNodes() error {
	nodes, err := oc.subnetRegistry.GetNodes()
	if err != nil {
		return err
	}

	for _, node := range nodes {
		_, err := oc.subnetRegistry.GetSubnet(node.Name)
		if err == nil {
			// subnet already exists, continue
			continue
		}
		err = oc.AddNode(node.Name, node.IP)
		if err != nil {
			return err
		}
	}
	return nil
}

func (oc *OvsController) AddNode(nodeName string, nodeIP string) error {
	sn, err := oc.subnetAllocator.GetNetwork()
	if err != nil {
		log.Errorf("Error creating network for node %s.", nodeName)
		return err
	}

	if nodeIP == "" || nodeIP == "127.0.0.1" {
		return fmt.Errorf("Invalid node IP")
	}

	subnet := &api.Subnet{
		NodeIP:   nodeIP,
		SubnetIP: sn.String(),
	}
	err = oc.subnetRegistry.CreateSubnet(nodeName, subnet)
	if err != nil {
		log.Errorf("Error writing subnet to etcd for node %s: %v", nodeName, sn)
		return err
	}
	return nil
}

func (oc *OvsController) DeleteNode(nodeName string) error {
	sub, err := oc.subnetRegistry.GetSubnet(nodeName)
	if err != nil {
		log.Errorf("Error fetching subnet for node %s for delete operation.", nodeName)
		return err
	}
	_, ipnet, err := net.ParseCIDR(sub.SubnetIP)
	if err != nil {
		log.Errorf("Error parsing subnet for node %s for deletion: %s", nodeName, sub.SubnetIP)
		return err
	}
	oc.subnetAllocator.ReleaseNetwork(ipnet)
	return oc.subnetRegistry.DeleteSubnet(nodeName)
}

func (oc *OvsController) syncWithMaster() error {
	return oc.subnetRegistry.CreateNode(oc.hostName, oc.localIP)
}

func (oc *OvsController) StartNode(sync, skipsetup bool, mtu uint) error {
	if sync {
		err := oc.syncWithMaster()
		if err != nil {
			log.Errorf("Failed to register with master: %v", err)
			return err
		}
	}
	err := oc.initSelfSubnet()
	if err != nil {
		log.Errorf("Failed to get subnet for this host: %v", err)
		return err
	}

	// call flow controller's setup
	if !skipsetup {
		// Assume we are working with IPv4
		containerNetwork, err := oc.subnetRegistry.GetContainerNetwork()
		if err != nil {
			log.Errorf("Failed to obtain ContainerNetwork: %v", err)
			return err
		}
		servicesNetwork, err := oc.subnetRegistry.GetServicesNetwork()
		if err != nil {
			log.Errorf("Failed to obtain ServicesNetwork: %v", err)
			return err
		}
		err = oc.flowController.Setup(oc.localSubnet.SubnetIP, containerNetwork, servicesNetwork, mtu)
		if err != nil {
			return err
		}
	}
	subnets, err := oc.subnetRegistry.GetSubnets()
	if err != nil {
		log.Errorf("Could not fetch existing subnets: %v", err)
	}
	for _, s := range subnets {
		oc.flowController.AddOFRules(s.NodeIP, s.SubnetIP, oc.localIP)
	}
	if _, ok := oc.flowController.(*multitenant.FlowController); ok {
		nslist, err := oc.subnetRegistry.GetNetNamespaces()
		if err != nil {
			return err
		}
		for _, ns := range nslist {
			oc.VNIDMap[ns.Name] = ns.NetID
		}
		go oc.watchVnids()

		services, err := oc.subnetRegistry.GetServices()
		if err != nil {
			return err
		}
		for _, svc := range services {
			oc.flowController.AddServiceOFRules(oc.VNIDMap[svc.Namespace], svc.IP, svc.Protocol, svc.Port)
		}
		go oc.watchServices()
	}
	go oc.watchCluster()

	if oc.ready != nil {
		close(oc.ready)
	}

	return err
}

func (oc *OvsController) watchVnids() {
	netNsEvent := make(chan *api.NetNamespaceEvent)
	stop := make(chan bool)
	go oc.subnetRegistry.WatchNetNamespaces(netNsEvent, stop)
	for {
		select {
		case ev := <-netNsEvent:
			switch ev.Type {
			case api.Added:
				oc.VNIDMap[ev.Name] = ev.NetID
			case api.Deleted:
				delete(oc.VNIDMap, ev.Name)
			}
		case <-oc.sig:
			log.Error("Signal received. Stopping watching of NetNamespaces.")
			stop <- true
			return
		}
	}
}

func (oc *OvsController) initSelfSubnet() error {
	// get subnet for self
	for {
		sub, err := oc.subnetRegistry.GetSubnet(oc.hostName)
		if err != nil {
			log.Errorf("Could not find an allocated subnet for node %s: %s. Waiting...", oc.hostName, err)
			time.Sleep(2 * time.Second)
			continue
		}
		oc.localSubnet = sub
		return nil
	}
}

func (oc *OvsController) watchNodes() {
	// watch latest?
	stop := make(chan bool)
	nodeEvent := make(chan *api.NodeEvent)
	go oc.subnetRegistry.WatchNodes(nodeEvent, stop)
	for {
		select {
		case ev := <-nodeEvent:
			switch ev.Type {
			case api.Added:
				sub, err := oc.subnetRegistry.GetSubnet(ev.Node.Name)
				if err != nil {
					// subnet does not exist already
					oc.AddNode(ev.Node.Name, ev.Node.IP)
				} else {
					// Current node IP is obtained from event, ev.NodeIP to
					// avoid cached/stale IP lookup by net.LookupIP()
					if sub.NodeIP != ev.Node.IP {
						err = oc.subnetRegistry.DeleteSubnet(ev.Node.Name)
						if err != nil {
							log.Errorf("Error deleting subnet for node %s, old ip %s", ev.Node.Name, sub.NodeIP)
							continue
						}
						sub.NodeIP = ev.Node.IP
						err = oc.subnetRegistry.CreateSubnet(ev.Node.Name, sub)
						if err != nil {
							log.Errorf("Error creating subnet for node %s, ip %s", ev.Node.Name, sub.NodeIP)
							continue
						}
					}
				}
			case api.Deleted:
				oc.DeleteNode(ev.Node.Name)
			}
		case <-oc.sig:
			log.Error("Signal received. Stopping watching of nodes.")
			stop <- true
			return
		}
	}
}

func (oc *OvsController) watchServices() {
	stop := make(chan bool)
	svcevent := make(chan *api.ServiceEvent)
	go oc.subnetRegistry.WatchServices(svcevent, stop)
	for {
		select {
		case ev := <-svcevent:
			netid := oc.VNIDMap[ev.Service.Namespace]
			switch ev.Type {
			case api.Added:
				oc.flowController.AddServiceOFRules(netid, ev.Service.IP, ev.Service.Protocol, ev.Service.Port)
			case api.Deleted:
				oc.flowController.DelServiceOFRules(netid, ev.Service.IP, ev.Service.Protocol, ev.Service.Port)
			}
		case <-oc.sig:
			log.Error("Signal received. Stopping watching of services.")
			stop <- true
			return
		}
	}
}

func (oc *OvsController) watchCluster() {
	stop := make(chan bool)
	clusterEvent := make(chan *api.SubnetEvent)
	go oc.subnetRegistry.WatchSubnets(clusterEvent, stop)
	for {
		select {
		case ev := <-clusterEvent:
			switch ev.Type {
			case api.Added:
				// add openflow rules
				oc.flowController.AddOFRules(ev.Subnet.NodeIP, ev.Subnet.SubnetIP, oc.localIP)
			case api.Deleted:
				// delete openflow rules meant for the node
				oc.flowController.DelOFRules(ev.Subnet.NodeIP, oc.localIP)
			}
		case <-oc.sig:
			stop <- true
			return
		}
	}
}

func (oc *OvsController) Stop() {
	close(oc.sig)
	//oc.sig <- struct{}{}
}

func GetNodeIP(nodeName string) (string, error) {
	ip := net.ParseIP(nodeName)
	if ip == nil {
		addrs, err := net.LookupIP(nodeName)
		if err != nil {
			log.Errorf("Failed to lookup IP address for node %s: %v", nodeName, err)
			return "", err
		}
		for _, addr := range addrs {
			if addr.String() != "127.0.0.1" {
				ip = addr
				break
			}
		}
	}
	if ip == nil || len(ip.String()) == 0 {
		return "", fmt.Errorf("Failed to obtain IP address from node name: %s", nodeName)
	}
	return ip.String(), nil
}
