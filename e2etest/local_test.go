// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package e2etest

import (
	"context"
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotexproject/iotex-core/blockchain"
	"github.com/iotexproject/iotex-core/blockchain/action"
	"github.com/iotexproject/iotex-core/config"
	"github.com/iotexproject/iotex-core/iotxaddress"
	"github.com/iotexproject/iotex-core/network"
	"github.com/iotexproject/iotex-core/network/node"
	"github.com/iotexproject/iotex-core/pkg/keypair"
	pb "github.com/iotexproject/iotex-core/proto"
	"github.com/iotexproject/iotex-core/server/itx"
	ta "github.com/iotexproject/iotex-core/test/testaddress"
	"github.com/iotexproject/iotex-core/testutil"
)

const (
	testDBPath    = "db.test"
	testDBPath2   = "db.test2"
	testTriePath  = "trie.test"
	testTriePath2 = "trie.test2"
)

func TestLocalCommit(t *testing.T) {
	require := require.New(t)

	testutil.CleanupPath(t, testTriePath)
	testutil.CleanupPath(t, testDBPath)

	blockchain.Gen.TotalSupply = uint64(50 << 22)
	blockchain.Gen.BlockReward = uint64(0)

	cfg, err := newTestConfig()
	require.Nil(err)

	// create server
	ctx := context.Background()
	svr := itx.NewServer(cfg)
	require.NoError(svr.Start(ctx))
	require.NotNil(svr.Blockchain())
	require.NoError(addTestingTsfBlocks(svr.Blockchain()))
	require.NotNil(svr.ActionPool())
	require.NotNil(svr.P2P())

	// create client
	cfg.Network.BootstrapNodes = []string{svr.P2P().Self().String()}
	p := network.NewOverlay(&cfg.Network)
	require.NotNil(p)
	require.NoError(p.Start(ctx))

	defer func() {
		require.Nil(p.Stop(ctx))
		require.Nil(svr.Stop(ctx))
		testutil.CleanupPath(t, testTriePath)
		testutil.CleanupPath(t, testDBPath)
	}()

	// check balance
	s, err := svr.Blockchain().StateByAddr(ta.Addrinfo["alfa"].RawAddress)
	require.Nil(err)
	change := s.Balance
	t.Logf("Alfa balance = %d", change)
	require.True(change.String() == "23")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["bravo"].RawAddress)
	require.Nil(err)
	beta := s.Balance
	t.Logf("Bravo balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "34")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["charlie"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Charlie balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "47")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["delta"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Delta balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "69")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["echo"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Echo balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "100")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["foxtrot"].RawAddress)
	require.Nil(err)
	fox := s.Balance
	t.Logf("Foxtrot balance = %d", fox)
	change.Add(change, fox)
	require.True(fox.String() == "5242883")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["producer"].RawAddress)
	require.Nil(err)
	test := s.Balance
	t.Logf("test balance = %d", test)
	change.Add(change, test)

	require.Equal(uint64(100000000), change.Uint64())
	t.Log("Total balance match")

	if beta.Sign() == 0 || fox.Sign() == 0 || test.Sign() == 0 {
		return
	}

	height, err := svr.Blockchain().TipHeight()
	require.Nil(err)
	require.True(height == 5)

	// transfer 1
	// C --> A
	s, _ = svr.Blockchain().StateByAddr(ta.Addrinfo["charlie"].RawAddress)
	tsf1, _ := action.NewTransfer(s.Nonce+1, big.NewInt(1), ta.Addrinfo["charlie"].RawAddress, ta.Addrinfo["alfa"].RawAddress)
	tsf1, _ = tsf1.Sign(ta.Addrinfo["charlie"])
	act1 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf1.ConvertToTransferPb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act1); err != nil {
			return false, err
		}
		tsf, _ := svr.ActionPool().PickActs()
		return len(tsf) == 1, nil
	})
	require.Nil(err)

	tsf, _ := svr.ActionPool().PickActs()
	blk1, err := svr.Blockchain().MintNewBlock(tsf, nil, ta.Addrinfo["producer"], "")
	hash1 := blk1.HashBlock()
	require.Nil(err)

	// transfer 2
	// F --> D
	s, _ = svr.Blockchain().StateByAddr(ta.Addrinfo["foxtrot"].RawAddress)
	tsf2, _ := action.NewTransfer(s.Nonce+1, big.NewInt(1), ta.Addrinfo["foxtrot"].RawAddress, ta.Addrinfo["delta"].RawAddress)
	tsf2, _ = tsf2.Sign(ta.Addrinfo["foxtrot"])
	blk2 := blockchain.NewBlock(0, height+2, hash1, []*action.Transfer{tsf2,
		action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)), ta.Addrinfo["producer"].RawAddress)}, nil)
	err = blk2.SignBlock(ta.Addrinfo["producer"])
	require.Nil(err)
	hash2 := blk2.HashBlock()
	act2 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf2.ConvertToTransferPb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act2); err != nil {
			return false, err
		}
		tsf, _ := svr.ActionPool().PickActs()
		return len(tsf) == 2, nil
	})
	require.Nil(err)

	// transfer 3
	// B --> B
	s, _ = svr.Blockchain().StateByAddr(ta.Addrinfo["bravo"].RawAddress)
	tsf3, _ := action.NewTransfer(s.Nonce+1, big.NewInt(1), ta.Addrinfo["bravo"].RawAddress, ta.Addrinfo["bravo"].RawAddress)
	tsf3, _ = tsf3.Sign(ta.Addrinfo["bravo"])
	blk3 := blockchain.NewBlock(0, height+3, hash2, []*action.Transfer{tsf3,
		action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)), ta.Addrinfo["producer"].RawAddress)}, nil)
	err = blk3.SignBlock(ta.Addrinfo["producer"])
	require.Nil(err)
	hash3 := blk3.HashBlock()
	act3 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf3.ConvertToTransferPb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act3); err != nil {
			return false, err
		}
		tsf, _ := svr.ActionPool().PickActs()
		return len(tsf) == 3, nil
	})
	require.Nil(err)

	// transfer 4
	// test --> E
	s, _ = svr.Blockchain().StateByAddr(ta.Addrinfo["producer"].RawAddress)
	tsf4, _ := action.NewTransfer(s.Nonce+1, big.NewInt(1), ta.Addrinfo["producer"].RawAddress, ta.Addrinfo["echo"].RawAddress)
	tsf4, _ = tsf4.Sign(ta.Addrinfo["producer"])
	blk4 := blockchain.NewBlock(0, height+4, hash3, []*action.Transfer{tsf4,
		action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)), ta.Addrinfo["producer"].RawAddress)}, nil)
	err = blk4.SignBlock(ta.Addrinfo["producer"])
	require.Nil(err)
	act4 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf4.ConvertToTransferPb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act4); err != nil {
			return false, err
		}
		tsf, _ := svr.ActionPool().PickActs()
		return len(tsf) == 4, nil
	})
	require.Nil(err)

	err = p.Broadcast(blk2.ConvertToBlockPb())
	require.NoError(err)
	err = p.Broadcast(blk4.ConvertToBlockPb())
	require.NoError(err)
	err = p.Broadcast(blk1.ConvertToBlockPb())
	require.NoError(err)
	err = p.Broadcast(blk3.ConvertToBlockPb())
	require.NoError(err)

	err = testutil.WaitUntil(10*time.Millisecond, 10*time.Second, func() (bool, error) {
		height, err := svr.Blockchain().TipHeight()
		if err != nil {
			return false, err
		}
		return int(height) == 9, nil
	})
	require.Nil(err)
	height, err = svr.Blockchain().TipHeight()
	require.Nil(err)
	require.Equal(9, int(height))

	// check balance
	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["alfa"].RawAddress)
	require.Nil(err)
	change = s.Balance
	t.Logf("Alfa balance = %d", change)
	require.True(change.String() == "24")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["bravo"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Bravo balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "34")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["charlie"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Charlie balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "46")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["delta"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Delta balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "70")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["echo"].RawAddress)
	require.Nil(err)
	beta = s.Balance
	t.Logf("Echo balance = %d", beta)
	change.Add(change, beta)
	require.True(beta.String() == "101")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["foxtrot"].RawAddress)
	require.Nil(err)
	fox = s.Balance
	t.Logf("Foxtrot balance = %d", fox)
	change.Add(change, fox)
	require.True(fox.String() == "5242882")

	s, err = svr.Blockchain().StateByAddr(ta.Addrinfo["producer"].RawAddress)
	require.Nil(err)
	test = s.Balance
	t.Logf("test balance = %d", test)
	change.Add(change, test)

	require.Equal(uint64(100000000), change.Uint64())
	t.Log("Total balance match")
}

func TestLocalSync(t *testing.T) {
	require := require.New(t)

	testutil.CleanupPath(t, testTriePath)
	testutil.CleanupPath(t, testDBPath)
	testutil.CleanupPath(t, testTriePath2)
	testutil.CleanupPath(t, testDBPath2)

	cfg, err := newTestConfig()
	require.Nil(err)

	// Create server
	ctx := context.Background()
	svr := itx.NewServer(cfg)
	require.Nil(svr.Start(ctx))
	require.NotNil(svr.Blockchain())
	require.Nil(addTestingTsfBlocks(svr.Blockchain()))

	blk, err := svr.Blockchain().GetBlockByHeight(1)
	require.Nil(err)
	hash1 := blk.HashBlock()
	blk, err = svr.Blockchain().GetBlockByHeight(2)
	require.Nil(err)
	hash2 := blk.HashBlock()
	blk, err = svr.Blockchain().GetBlockByHeight(3)
	require.Nil(err)
	hash3 := blk.HashBlock()
	blk, err = svr.Blockchain().GetBlockByHeight(4)
	require.Nil(err)
	hash4 := blk.HashBlock()
	blk, err = svr.Blockchain().GetBlockByHeight(5)
	require.Nil(err)
	hash5 := blk.HashBlock()

	p2 := svr.P2P()
	require.NotNil(p2)

	testutil.CleanupPath(t, testTriePath2)
	testutil.CleanupPath(t, testDBPath2)

	cfg.Chain.TrieDBPath = testTriePath2
	cfg.Chain.ChainDBPath = testDBPath2

	// Create client
	cfg.NodeType = config.FullNodeType
	cfg.Network.BootstrapNodes = []string{svr.P2P().Self().String()}
	cli := itx.NewServer(cfg)
	require.Nil(cli.Start(ctx))
	require.NotNil(cli.Blockchain())
	require.NotNil(cli.P2P())

	defer func() {
		require.Nil(cli.Stop(ctx))
		require.Nil(svr.Stop(ctx))
		testutil.CleanupPath(t, testTriePath)
		testutil.CleanupPath(t, testDBPath)
		testutil.CleanupPath(t, testTriePath2)
		testutil.CleanupPath(t, testDBPath2)
	}()

	// P1 download 4 blocks from P2
	err = cli.P2P().Tell(node.NewTCPNode(svr.P2P().Self().String()), &pb.BlockSync{Start: 1, End: 5})
	require.NoError(err)
	check := testutil.CheckCondition(func() (bool, error) {
		blk1, err := cli.Blockchain().GetBlockByHeight(1)
		if err != nil {
			return false, nil
		}
		blk2, err := cli.Blockchain().GetBlockByHeight(2)
		if err != nil {
			return false, nil
		}
		blk3, err := cli.Blockchain().GetBlockByHeight(3)
		if err != nil {
			return false, nil
		}
		blk4, err := cli.Blockchain().GetBlockByHeight(4)
		if err != nil {
			return false, nil
		}
		blk5, err := cli.Blockchain().GetBlockByHeight(5)
		if err != nil {
			return false, nil
		}
		return hash1 == blk1.HashBlock() &&
			hash2 == blk2.HashBlock() &&
			hash3 == blk3.HashBlock() &&
			hash4 == blk4.HashBlock() &&
			hash5 == blk5.HashBlock(), nil
	})
	err = testutil.WaitUntil(time.Millisecond*10, time.Second*5, check)
	require.Nil(err)

	// verify 4 received blocks
	blk, err = cli.Blockchain().GetBlockByHeight(1)
	require.Nil(err)
	require.Equal(hash1, blk.HashBlock())
	blk, err = cli.Blockchain().GetBlockByHeight(2)
	require.Nil(err)
	require.Equal(hash2, blk.HashBlock())
	blk, err = cli.Blockchain().GetBlockByHeight(3)
	require.Nil(err)
	require.Equal(hash3, blk.HashBlock())
	blk, err = cli.Blockchain().GetBlockByHeight(4)
	require.Nil(err)
	require.Equal(hash4, blk.HashBlock())
	blk, err = cli.Blockchain().GetBlockByHeight(5)
	require.Nil(err)
	require.Equal(hash5, blk.HashBlock())
	t.Log("4 blocks received correctly")
}

func TestVoteLocalCommit(t *testing.T) {
	require := require.New(t)

	testutil.CleanupPath(t, testTriePath)
	testutil.CleanupPath(t, testDBPath)

	cfg, err := newTestConfig()
	cfg.Chain.NumCandidates = 2
	require.Nil(err)

	blockchain.Gen.TotalSupply = uint64(50 << 22)
	blockchain.Gen.BlockReward = uint64(0)

	// create node
	ctx := context.Background()
	svr := itx.NewServer(cfg)
	require.Nil(svr.Start(ctx))
	require.NotNil(svr.Blockchain())
	require.Nil(addTestingTsfBlocks(svr.Blockchain()))
	require.NotNil(svr.ActionPool())

	cfg.Network.BootstrapNodes = []string{svr.P2P().Self().String()}
	p := network.NewOverlay(&cfg.Network)
	require.NotNil(p)
	require.NoError(p.Start(ctx))

	defer func() {
		require.Nil(p.Stop(ctx))
		require.Nil(svr.Stop(ctx))
		testutil.CleanupPath(t, testTriePath)
		testutil.CleanupPath(t, testDBPath)
	}()

	height, err := svr.Blockchain().TipHeight()
	require.Nil(err)
	require.True(height == 5)

	// Add block 1
	// Alfa, Bravo and Charlie selfnomination
	tsf1, err := action.NewTransfer(7, big.NewInt(20000000), ta.Addrinfo["producer"].RawAddress, ta.Addrinfo["alfa"].RawAddress)
	require.Nil(err)
	tsf1, err = tsf1.Sign(ta.Addrinfo["producer"])
	require.Nil(err)
	tsf2, err := action.NewTransfer(8, big.NewInt(20000000), ta.Addrinfo["producer"].RawAddress, ta.Addrinfo["bravo"].RawAddress)
	require.Nil(err)
	tsf2, err = tsf2.Sign(ta.Addrinfo["producer"])
	require.Nil(err)
	tsf3, err := action.NewTransfer(9, big.NewInt(20000000), ta.Addrinfo["producer"].RawAddress, ta.Addrinfo["charlie"].RawAddress)
	require.Nil(err)
	tsf3, err = tsf3.Sign(ta.Addrinfo["producer"])
	require.Nil(err)
	tsf4, err := action.NewTransfer(10, big.NewInt(20000000), ta.Addrinfo["producer"].RawAddress, ta.Addrinfo["delta"].RawAddress)
	require.Nil(err)
	tsf4, err = tsf4.Sign(ta.Addrinfo["producer"])
	require.Nil(err)
	vote1, err := newSignedVote(1, ta.Addrinfo["alfa"], ta.Addrinfo["alfa"])
	require.Nil(err)
	vote2, err := newSignedVote(1, ta.Addrinfo["bravo"], ta.Addrinfo["bravo"])
	require.Nil(err)
	vote3, err := newSignedVote(6, ta.Addrinfo["charlie"], ta.Addrinfo["charlie"])
	require.Nil(err)
	act1 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote1.ConvertToVotePb()}}
	act2 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote2.ConvertToVotePb()}}
	act3 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote3.ConvertToVotePb()}}
	acttsf1 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf1.ConvertToTransferPb()}}
	acttsf2 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf2.ConvertToTransferPb()}}
	acttsf3 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf3.ConvertToTransferPb()}}
	acttsf4 := &pb.ActionPb{Action: &pb.ActionPb_Transfer{tsf4.ConvertToTransferPb()}}

	err = testutil.WaitUntil(10*time.Millisecond, 5*time.Second, func() (bool, error) {
		if err := p.Broadcast(act1); err != nil {
			return false, err
		}
		if err := p.Broadcast(act2); err != nil {
			return false, err
		}
		if err := p.Broadcast(act3); err != nil {
			return false, err
		}
		if err := p.Broadcast(acttsf1); err != nil {
			return false, err
		}
		if err := p.Broadcast(acttsf2); err != nil {
			return false, err
		}
		if err := p.Broadcast(acttsf3); err != nil {
			return false, err
		}
		if err := p.Broadcast(acttsf4); err != nil {
			return false, err
		}
		transfer, votes := svr.ActionPool().PickActs()
		return len(votes)+len(transfer) == 7, nil
	})
	require.Nil(err)

	transfers, votes := svr.ActionPool().PickActs()
	blk1, err := svr.Blockchain().MintNewBlock(transfers, votes, ta.Addrinfo["producer"], "")
	hash1 := blk1.HashBlock()
	require.Nil(err)

	err = p.Broadcast(blk1.ConvertToBlockPb())
	require.NoError(err)
	err = testutil.WaitUntil(10*time.Millisecond, 5*time.Second, func() (bool, error) {
		height, err := svr.Blockchain().TipHeight()
		if err != nil {
			return false, err
		}
		return int(height) == 6, nil
	})
	require.NoError(err)
	tipheight, err := svr.Blockchain().TipHeight()
	require.Nil(err)
	require.Equal(6, int(tipheight))

	// Add block 2
	// Vote A -> B, C -> A
	vote4, err := newSignedVote(2, ta.Addrinfo["alfa"], ta.Addrinfo["bravo"])
	require.Nil(err)
	vote5, err := newSignedVote(7, ta.Addrinfo["charlie"], ta.Addrinfo["alfa"])
	require.Nil(err)
	blk2 := blockchain.NewBlock(0, height+2, hash1,
		[]*action.Transfer{action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)),
			ta.Addrinfo["producer"].RawAddress)}, []*action.Vote{vote4, vote5})
	err = blk2.SignBlock(ta.Addrinfo["producer"])
	hash2 := blk2.HashBlock()
	require.Nil(err)
	act4 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote4.ConvertToVotePb()}}
	act5 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote5.ConvertToVotePb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act4); err != nil {
			return false, err
		}
		if err := p.Broadcast(act5); err != nil {
			return false, err
		}
		_, votes := svr.ActionPool().PickActs()
		return len(votes) == 2, nil
	})
	require.Nil(err)

	err = p.Broadcast(blk2.ConvertToBlockPb())
	require.NoError(err)

	err = testutil.WaitUntil(10*time.Millisecond, 5*time.Second, func() (bool, error) {
		height, err := svr.Blockchain().TipHeight()
		if err != nil {
			return false, err
		}
		return int(height) == 7, nil
	})
	require.Nil(err)
	tipheight, err = svr.Blockchain().TipHeight()
	require.Nil(err)
	require.Equal(7, int(tipheight))

	h, candidates := svr.Blockchain().Candidates()
	candidatesAddr := make([]string, len(candidates))
	for i, can := range candidates {
		candidatesAddr[i] = can.Address
	}
	require.Equal(7, int(h))
	require.Equal(2, len(candidates))

	sort.Sort(sort.StringSlice(candidatesAddr))
	require.Equal(ta.Addrinfo["alfa"].RawAddress, candidatesAddr[0])
	require.Equal(ta.Addrinfo["bravo"].RawAddress, candidatesAddr[1])

	// Add block 3
	// D self nomination
	vote6, err := action.NewVote(uint64(5), ta.Addrinfo["delta"].RawAddress, ta.Addrinfo["delta"].RawAddress)
	require.NoError(err)
	vote6, err = vote6.Sign(ta.Addrinfo["delta"])
	require.Nil(err)
	blk3 := blockchain.NewBlock(0, height+3, hash2,
		[]*action.Transfer{action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)),
			ta.Addrinfo["producer"].RawAddress)}, []*action.Vote{vote6})
	err = blk3.SignBlock(ta.Addrinfo["producer"])
	hash3 := blk3.HashBlock()
	require.Nil(err)
	act6 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote6.ConvertToVotePb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act6); err != nil {
			return false, err
		}
		_, votes := svr.ActionPool().PickActs()
		return len(votes) == 1, nil
	})
	require.Nil(err)

	err = p.Broadcast(blk3.ConvertToBlockPb())
	require.NoError(err)

	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		height, err := svr.Blockchain().TipHeight()
		if err != nil {
			return false, err
		}
		return int(height) == 8, nil
	})
	require.Nil(err)
	tipheight, err = svr.Blockchain().TipHeight()
	require.Nil(err)
	require.Equal(8, int(tipheight))

	h, candidates = svr.Blockchain().Candidates()
	candidatesAddr = make([]string, len(candidates))
	for i, can := range candidates {
		candidatesAddr[i] = can.Address
	}
	require.Equal(8, int(h))
	require.Equal(2, len(candidates))

	sort.Sort(sort.StringSlice(candidatesAddr))
	require.Equal(ta.Addrinfo["bravo"].RawAddress, candidatesAddr[0])
	require.Equal(ta.Addrinfo["delta"].RawAddress, candidatesAddr[1])

	// Add block 4
	// Unvote B
	vote7, err := action.NewVote(uint64(2), ta.Addrinfo["bravo"].RawAddress, "")
	require.NoError(err)
	vote7, err = vote7.Sign(ta.Addrinfo["bravo"])
	require.Nil(err)
	blk4 := blockchain.NewBlock(0, height+4, hash3,
		[]*action.Transfer{action.NewCoinBaseTransfer(big.NewInt(int64(blockchain.Gen.BlockReward)),
			ta.Addrinfo["producer"].RawAddress)}, []*action.Vote{vote7})
	err = blk4.SignBlock(ta.Addrinfo["producer"])
	require.Nil(err)
	act7 := &pb.ActionPb{Action: &pb.ActionPb_Vote{vote7.ConvertToVotePb()}}
	err = testutil.WaitUntil(10*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := p.Broadcast(act7); err != nil {
			return false, err
		}
		_, votes := svr.ActionPool().PickActs()
		return len(votes) == 1, nil
	})
	require.Nil(err)

	err = p.Broadcast(blk4.ConvertToBlockPb())
	require.NoError(err)

	err = testutil.WaitUntil(10*time.Millisecond, 5*time.Second, func() (bool, error) {
		height, err := svr.Blockchain().TipHeight()
		if err != nil {
			return false, err
		}
		return int(height) == 9, nil
	})
	require.Nil(err)
	tipheight, err = svr.Blockchain().TipHeight()
	require.Nil(err)
	require.Equal(9, int(tipheight))

	h, candidates = svr.Blockchain().Candidates()
	candidatesAddr = make([]string, len(candidates))
	for i, can := range candidates {
		candidatesAddr[i] = can.Address
	}
	require.Equal(9, int(h))
	require.Equal(2, len(candidates))

	sort.Sort(sort.StringSlice(candidatesAddr))
	require.Equal(ta.Addrinfo["alfa"].RawAddress, candidatesAddr[0])
	require.Equal(ta.Addrinfo["delta"].RawAddress, candidatesAddr[1])
}

func newSignedVote(nonce int, from *iotxaddress.Address, to *iotxaddress.Address) (*action.Vote, error) {
	vote, err := action.NewVote(uint64(nonce), from.RawAddress, to.RawAddress)
	if err != nil {
		return nil, err
	}
	vote, err = vote.Sign(from)
	if err != nil {
		return nil, err
	}
	return vote, nil
}

func newTestConfig() (*config.Config, error) {
	cfg := config.Default
	cfg.Chain.TrieDBPath = testTriePath
	cfg.Chain.ChainDBPath = testDBPath
	cfg.Consensus.Scheme = config.NOOPScheme
	cfg.Network.Port = 0
	cfg.Explorer.Port = 0
	addr, err := iotxaddress.NewAddress(true, iotxaddress.ChainID)
	if err != nil {
		return nil, err
	}
	cfg.Chain.ProducerPubKey = keypair.EncodePublicKey(addr.PublicKey)
	cfg.Chain.ProducerPrivKey = keypair.EncodePrivateKey(addr.PrivateKey)
	return &cfg, nil
}
