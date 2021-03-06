// +build mongodb

package digest

import (
	"fmt"
	"io"
	"net/url"
	"testing"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/base/key"
	"github.com/spikeekips/mitum/base/node"
	"github.com/spikeekips/mitum/network"
	"github.com/spikeekips/mitum/util"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testHandlerNodeInfo struct {
	baseTestHandlers
}

func (t *testHandlerNodeInfo) newNode(name string) (base.Node, network.ConnInfo) {
	addr, err := base.NewStringAddress(name)
	t.NoError(err)

	no := node.NewBaseV0(addr, key.MustNewBTCPrivatekey().Publickey())
	u, _ := url.Parse(fmt.Sprintf("https://%s:443", name))
	connInfo := network.NewHTTPConnInfo(u, true)

	return no, connInfo
}

func (t *testHandlerNodeInfo) TestBasic() {
	st, _ := t.Database()

	blk, err := block.NewTestBlockV0(base.Height(33), base.Round(0), valuehash.RandomSHA256(), valuehash.RandomSHA256())
	t.NoError(err)

	local := node.RandomNode("n0")

	n1, n1ConnInfo := t.newNode("n1")
	n2, n2ConnInfo := t.newNode("n2")

	nodes := []network.RemoteNode{
		network.NewRemoteNode(n1, n1ConnInfo),
		network.NewRemoteNode(n2, n2ConnInfo),
	}

	policy := map[string]interface{}{"showme": 1}

	ni := network.NewNodeInfoV0(
		local,
		t.networkID,
		base.StateBooting,
		blk.Manifest(),
		util.Version("0.1.1"),
		"quic://local",
		policy,
		nodes,
		nil,
	)

	handlers := t.handlers(st, DummyCache{})

	handlers.SetNodeInfoHandler(func() (network.NodeInfo, error) {
		return NewNodeInfo(ni), nil
	})

	self, err := handlers.router.Get("root").URL()
	t.NoError(err)

	w := t.requestOK(handlers, "GET", self.Path, nil)

	b, err := io.ReadAll(w.Result().Body)
	t.NoError(err)

	hal := t.loadHal(b)

	uni, err := network.DecodeNodeInfo(hal.RawInterface(), t.JSONEnc)
	t.NoError(err)

	t.compareNodeInfo(ni, uni)
}

func (t *testHandlerNodeInfo) compareNodeInfo(a, b network.NodeInfo) {
	t.True(a.Address().Equal(b.Address()))
	t.True(a.Publickey().Equal(b.Publickey()))
	t.Equal(a.NetworkID(), b.NetworkID())
	t.Equal(a.Version(), b.Version())
	t.Equal(a.URL(), b.URL())

	t.Equal(len(a.Policy()), len(b.Policy()))
	{
		ab, err := jsonenc.Marshal(a.Policy())
		t.NoError(err)
		bb, err := jsonenc.Marshal(b.Policy())
		t.NoError(err)
		t.Equal(ab, bb)
	}

	t.Equal(len(a.Nodes()), len(b.Nodes()))
	for i := range a.Nodes() {
		an := a.Nodes()[i]
		bn := b.Nodes()[i]

		t.True(an.Address.Equal(bn.Address))
		t.True(an.Publickey.Equal(bn.Publickey))
		t.Equal(an.URL, bn.URL)
		t.Equal(an.Insecure, bn.Insecure)
	}
}

func TestHandlerNodeInfo(t *testing.T) {
	suite.Run(t, new(testHandlerNodeInfo))
}
