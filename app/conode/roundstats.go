package main

import (
	"github.com/dedis/cothority/lib/conode"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/sign"
)

/*
ConodeStats implements a simple module that shows some statistics about the
actual connection.
*/

// The name type of this round implementation
const RoundStatsType = "conodestats"

type RoundStats struct {
	*conode.RoundStamperListener
}

func init() {
	sign.RegisterRoundFactory(RoundStatsType,
		func(node *sign.Node) sign.Round {
			return NewRoundStats(node)
		})
}

func NewRoundStats(node *sign.Node) *RoundStats {
	round := &RoundStats{}
	round.RoundStamperListener = conode.NewRoundStamperListener(node)
	round.Type = RoundStatsType
	return round
}

func (round *RoundStats) Commitment(in []*sign.SigningMessage, out *sign.SigningMessage) error {
	err := round.RoundStamperListener.Commitment(in, out)
	return err
}

func (round *RoundStats) SignatureBroadcast(in *sign.SigningMessage, out []*sign.SigningMessage) error {
	err := round.RoundStamperListener.SignatureBroadcast(in, out)
	dbg.Lvlf1("Round %d with %d messages (including children) - %d since start.",
		round.RoundNbr, round.RoundMessages, in.SBm.Messages)
	return err
}
