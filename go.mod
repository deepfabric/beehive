module github.com/deepfabric/beehive

go 1.13

require (
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/coreos/etcd v3.3.17+incompatible
	github.com/deepfabric/prophet v0.0.0-20191118055338-af29aab59eed
	github.com/fagongzi/goetty v1.3.1
	github.com/fagongzi/log v0.0.0-20191106015352-59d362b5908d
	github.com/fagongzi/util v0.0.0-20191031020235-c0f29a56724d
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/google/btree v1.0.0
	github.com/prometheus/client_golang v1.2.1
	github.com/shirou/gopsutil v2.19.9+incompatible
	github.com/stretchr/testify v1.4.0
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
)

replace github.com/coreos/etcd => github.com/deepfabric/etcd v3.3.17+incompatible
