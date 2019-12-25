package rpc

import (
	"bytes"
	"encoding/hex"
	"github.com/kaspanet/kaspad/rpcmodel"
	"github.com/kaspanet/kaspad/util/daghash"
)

// handleGetHeaders implements the getHeaders command.
func handleGetHeaders(s *Server, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	c := cmd.(*rpcmodel.GetHeadersCmd)

	startHash := &daghash.ZeroHash
	if c.StartHash != "" {
		err := daghash.Decode(startHash, c.StartHash)
		if err != nil {
			return nil, rpcDecodeHexError(c.StopHash)
		}
	}
	stopHash := &daghash.ZeroHash
	if c.StopHash != "" {
		err := daghash.Decode(stopHash, c.StopHash)
		if err != nil {
			return nil, rpcDecodeHexError(c.StopHash)
		}
	}
	headers, err := s.cfg.SyncMgr.GetBlueBlocksHeadersBetween(startHash, stopHash)
	if err != nil {
		return nil, &rpcmodel.RPCError{
			Code:    rpcmodel.ErrRPCMisc,
			Message: err.Error(),
		}
	}

	// Return the serialized block headers as hex-encoded strings.
	hexBlockHeaders := make([]string, len(headers))
	var buf bytes.Buffer
	for i, h := range headers {
		err := h.Serialize(&buf)
		if err != nil {
			return nil, internalRPCError(err.Error(),
				"Failed to serialize block header")
		}
		hexBlockHeaders[i] = hex.EncodeToString(buf.Bytes())
		buf.Reset()
	}
	return hexBlockHeaders, nil
}