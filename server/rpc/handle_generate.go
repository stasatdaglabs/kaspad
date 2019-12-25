package rpc

import (
	"fmt"
	"github.com/kaspanet/kaspad/config"
	"github.com/kaspanet/kaspad/rpcmodel"
)

// handleGenerate handles generate commands.
func handleGenerate(s *Server, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	// Respond with an error if there are no addresses to pay the
	// created blocks to.
	if len(config.ActiveConfig().MiningAddrs) == 0 {
		return nil, &rpcmodel.RPCError{
			Code: rpcmodel.ErrRPCInternal.Code,
			Message: "No payment addresses specified " +
				"via --miningaddr",
		}
	}

	if config.ActiveConfig().SubnetworkID != nil {
		return nil, &rpcmodel.RPCError{
			Code:    rpcmodel.ErrRPCInvalidRequest.Code,
			Message: "`generate` is not supported on partial nodes.",
		}
	}

	// Respond with an error if there's virtually 0 chance of mining a block
	// with the CPU.
	if !s.cfg.DAGParams.GenerateSupported {
		return nil, &rpcmodel.RPCError{
			Code: rpcmodel.ErrRPCDifficulty,
			Message: fmt.Sprintf("No support for `generate` on "+
				"the current network, %s, as it's unlikely to "+
				"be possible to mine a block with the CPU.",
				s.cfg.DAGParams.Net),
		}
	}

	c := cmd.(*rpcmodel.GenerateCmd)

	// Respond with an error if the client is requesting 0 blocks to be generated.
	if c.NumBlocks == 0 {
		return nil, &rpcmodel.RPCError{
			Code:    rpcmodel.ErrRPCInternal.Code,
			Message: "Please request a nonzero number of blocks to generate.",
		}
	}

	// Create a reply
	reply := make([]string, c.NumBlocks)

	blockHashes, err := s.cfg.CPUMiner.GenerateNBlocks(c.NumBlocks)
	if err != nil {
		return nil, &rpcmodel.RPCError{
			Code:    rpcmodel.ErrRPCInternal.Code,
			Message: err.Error(),
		}
	}

	// Mine the correct number of blocks, assigning the hex representation of the
	// hash of each one to its place in the reply.
	for i, hash := range blockHashes {
		reply[i] = hash.String()
	}

	return reply, nil
}