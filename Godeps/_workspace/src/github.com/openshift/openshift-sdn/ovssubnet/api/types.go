package api

type EventType string

const (
	Added   EventType = "ADDED"
	Deleted EventType = "DELETED"
)

type SubnetRegistry interface {
	InitSubnets() error
	GetSubnets() ([]Subnet, error)
	GetSubnet(nodeName string) (*Subnet, error)
	DeleteSubnet(nodeName string) error
	CreateSubnet(sn string, sub *Subnet) error
	WatchSubnets(receiver chan *SubnetEvent, stop chan bool) error

	InitNodes() error
	GetNodes() ([]Node, error)
	CreateNode(nodeName string, data string) error
	WatchNodes(receiver chan *NodeEvent, stop chan bool) error

	WriteNetworkConfig(network string, subnetLength uint, serviceNetwork string) error
	GetContainerNetwork() (string, error)
	GetSubnetLength() (uint64, error)
	CheckEtcdIsAlive(seconds uint64) bool

	GetNamespaces() ([]string, error)
	WatchNamespaces(receiver chan *NamespaceEvent, stop chan bool) error

	WatchNetNamespaces(receiver chan *NetNamespaceEvent, stop chan bool) error
	GetNetNamespaces() ([]NetNamespace, error)
	GetNetNamespace(name string) (NetNamespace, error)
	WriteNetNamespace(name string, id uint) error
	DeleteNetNamespace(name string) error

	GetServicesNetwork() (string, error)
	GetServices() ([]Service, error)
	WatchServices(receiver chan *ServiceEvent, stop chan bool) error
}

type Subnet struct {
	NodeIP   string
	SubnetIP string
}

type SubnetEvent struct {
	Type     EventType
	NodeName string
	Subnet   Subnet
}

type Node struct {
	Name string
	IP   string
}

type NodeEvent struct {
	Type EventType
	Node Node
}

type NetNamespace struct {
	Name  string
	NetID uint
}

type NetNamespaceEvent struct {
	Type  EventType
	Name  string
	NetID uint
}

type NamespaceEvent struct {
	Type EventType
	Name string
}

type ServiceProtocol string

const (
	TCP ServiceProtocol = "TCP"
	UDP ServiceProtocol = "UDP"
)

type Service struct {
	Name      string
	Namespace string
	IP        string
	Protocol  ServiceProtocol
	Port      uint
}

type ServiceEvent struct {
	Type    EventType
	Service Service
}
