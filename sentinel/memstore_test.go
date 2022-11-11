package sentinel

import (
	"arkeo/common"
	"arkeo/x/arkeo/types"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "gopkg.in/check.v1"
)

type MemStoreSuite struct {
	server *httptest.Server
}

var _ = Suite(&MemStoreSuite{})

func (s *MemStoreSuite) SetUpTest(c *C) {
	s.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasPrefix(req.RequestURI, "/contract/cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu/arkeo-mainnet/cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu"):
			httpTestHandler(c, rw, `
			{
				"provider_pub_key": "cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu",
				"chain": 1,
				"client": "cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu",
				"delegate": "cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu",
				"type": 0,
				"height": 15,
				"duration": 100,
				"rate": 3,
				"deposit": "500",
				"paid": "0",
				"nonce": 9,
				"closed_height": 0
			}`)
		default:
			fmt.Println(req.RequestURI)
			panic("could not serve request")
		}
	}))
}

func httpTestHandler(c *C, rw http.ResponseWriter, content string) {
	if content == "500" {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	if _, err := rw.Write([]byte(content)); err != nil {
		c.Fatal(err)
	}
}

func (s *MemStoreSuite) TestMemStore(c *C) {
	var err error
	baseURL := fmt.Sprintf("http://%s/contract", s.server.Listener.Addr().String())
	mem := NewMemStore(baseURL)

	c.Check(mem.Key("foo", "bar", "baz"), Equals, "foo/bar/baz")

	mem.SetHeight(30)
	c.Check(mem.GetHeight(), Equals, int64(30))

	pk1 := types.GetRandomPubKey()
	pk2 := types.GetRandomPubKey()
	contract := types.NewContract(pk1, common.Chain(0), pk2)
	contract.Height = 4
	contract.Duration = 100
	key := mem.Key(pk1.String(), common.Chain(0).String(), pk2.String())

	mem.Put(key, contract)

	contract, err = mem.Get(key)
	c.Assert(err, IsNil)
	c.Check(contract.Height, Equals, int64(4))

	// fetch from server
	pk, err := common.NewPubKey("cosmospub1addwnpepqg3523h7e7ggeh6na2lsde6s394tqxnvufsz0urld6zwl8687ue9c3dasgu")
	c.Assert(err, IsNil)

	key = mem.Key(pk.String(), "arkeo-mainnet", pk.String())
	contract, err = mem.Get(key)
	c.Assert(err, IsNil)
	c.Check(contract.Rate, Equals, int64(3))
}
