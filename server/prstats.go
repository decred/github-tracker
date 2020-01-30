package server

import (
	"encoding/binary"
)

func (s *Server) PRStats(org, repo string, prNumber int) error {
	var prNum [8]byte
	binary.LittleEndian.PutUint64(prNum[:], uint64(prNumber))
	return nil
}
