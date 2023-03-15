package main

import (
	"fmt"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/urfave/cli/v2"
)

var verkleStatsCommand = &cli.Command{
	Action: verkleStats,
	Name:   "verkle-stats",
	Flags:  []cli.Flag{utils.DataDirFlag},
	Usage:  "Print stats of the current Verkle Tree state",
}

const (
	internalNodeType = 1
	leafNodeType     = 2
)

type depthStat struct {
	totalCount        uint64
	internalNodeCount uint64
	leafNodeCount     uint64

	totalBytes uint64
}

func verkleStats(ctx *cli.Context) error {
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	db := utils.MakeChainDatabase(ctx, stack, true)
	defer db.Close()

	it := db.NewIterator([]byte("vkt-"), nil)

	depthStats := make([]depthStat, 10)
	for it.Next() {
		nodeDepth := 0
		if len(it.Key()) < 4+8 {
			nodeDepth = len(it.Key()) - 4
		} else if len(it.Key()) == 4+8 {
			nodeDepth = 0
		} else {
			panic("invalid key")
		}
		nodeType := it.Value()[0]

		depthStats[nodeDepth].totalBytes += uint64(len(it.Value()))
		depthStats[nodeDepth].totalCount++
		if nodeType == internalNodeType {
			depthStats[nodeDepth].internalNodeCount++
		} else if nodeType == leafNodeType {
			depthStats[nodeDepth].leafNodeCount++
		}
	}

	for depth, stats := range depthStats {
		if stats.totalCount == 0 {
			continue
		}
		fmt.Printf("Depth %d -- Total nodes %d (Internal: %d, Leaf: %d), Bytes: %d KiB\n", depth, stats.totalCount, stats.internalNodeCount, stats.leafNodeCount, stats.totalBytes/1024)
	}

	return nil
}
