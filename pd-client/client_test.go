package pd

import (
	"flag"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	. "github.com/pingcap/check"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/kvproto/pkg/pdpb"
	"github.com/pingcap/pd/server"
	"github.com/pingcap/pd/util"
	"github.com/twinj/uuid"
)

func TestClient(t *testing.T) {
	TestingT(t)
}

var (
	testEtcd = flag.String("etcd", "127.0.0.1:2379", "Etcd endpoints, separated by comma")
)

var _ = Suite(&testClientSuite{})

var (
	// Note: IDs below are entirely arbitrary. They are only for checking
	// whether GetRegion/GetStore works.
	// If we alloc ID in client in the future, these IDs must be updated.
	clusterID = uint64(time.Now().Unix())
	store     = &metapb.Store{
		Id:      proto.Uint64(1),
		Address: proto.String("localhost"),
	}
	peer = &metapb.Peer{
		Id:      proto.Uint64(2),
		StoreId: proto.Uint64(store.GetId()),
	}
	region = &metapb.Region{
		Id: proto.Uint64(3),
		RegionEpoch: &metapb.RegionEpoch{
			ConfVer: proto.Uint64(1),
			Version: proto.Uint64(1),
		},
		Peers: []*metapb.Peer{peer},
	}
)

type testClientSuite struct {
	srv    *server.Server
	client Client
}

func (s *testClientSuite) SetUpSuite(c *C) {
	s.srv = newServer(c, 1234, "/pd-test")

	// wait for srv to become leader
	time.Sleep(time.Second * 3)

	bootstrapServer(c, 1234)

	var err error
	s.client, err = NewClient(strings.Split(*testEtcd, ","), "/pd-test", clusterID)
	c.Assert(err, IsNil)
}

func (s *testClientSuite) TearDownSuite(c *C) {
	s.client.Close()
	s.srv.Close()
}

func newServer(c *C, port int, root string) *server.Server {
	cfg := &server.Config{
		Addr:        fmt.Sprintf("127.0.0.1:%d", port),
		EtcdAddrs:   strings.Split(*testEtcd, ","),
		RootPath:    root,
		LeaderLease: 1,
	}
	s, err := server.NewServer(cfg)
	c.Assert(err, IsNil)

	go s.Run()
	return s
}

func bootstrapServer(c *C, port int) {
	req := pdpb.Request{
		Header: &pdpb.RequestHeader{
			Uuid:      uuid.NewV4().Bytes(),
			ClusterId: proto.Uint64(clusterID),
		},
		CmdType: pdpb.CommandType_Bootstrap.Enum(),
		Bootstrap: &pdpb.BootstrapRequest{
			Store:  store,
			Region: region,
		},
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	c.Assert(err, IsNil)
	err = util.WriteMessage(conn, 0, &req)
	c.Assert(err, IsNil)

	var rsp pdpb.Response
	_, err = util.ReadMessage(conn, &rsp)
	c.Assert(err, IsNil)
}

func (s *testClientSuite) TestTSO(c *C) {
	var tss []int64
	for i := 0; i < 100; i++ {
		p, l, err := s.client.GetTS()
		c.Assert(err, IsNil)
		tss = append(tss, p<<18+l)
	}

	var last int64
	for _, ts := range tss {
		c.Assert(ts, Greater, last)
		last = ts
	}
}

func (s *testClientSuite) TestGetRegion(c *C) {
	r, err := s.client.GetRegion([]byte("a"))
	c.Assert(err, IsNil)
	c.Assert(r, DeepEquals, region)
}

func (s *testClientSuite) TestGetStore(c *C) {
	n, err := s.client.GetStore(store.GetId())
	c.Assert(err, IsNil)
	c.Assert(n, DeepEquals, store)
}
