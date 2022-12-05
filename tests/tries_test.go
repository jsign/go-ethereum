package tests

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/gballet/go-verkle"
)

func BenchmarkTriesRandom(b *testing.B) {
	numAccounts := []int{1_000, 5_000, 10_000}

	for _, numAccounts := range numAccounts {
		rs := rand.New(rand.NewSource(42))
		accounts := getRandomStateAccounts(rs, numAccounts)

		b.Run(fmt.Sprintf("MPT/%d accounts", numAccounts), func(b *testing.B) {
			trie, _ := trie.NewStateTrie(trie.TrieID(common.Hash{}), trie.NewDatabase(memorydb.New()))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for k := 0; k < len(accounts); k++ {
					trie.TryUpdateAccount(accounts[k].address[:], &accounts[k].stateAccount)
				}
				trie.Commit(true)
			}
		})
		b.Run(fmt.Sprintf("VKT/%d accounts", numAccounts), func(b *testing.B) {
			// Warmup VKT configuration
			trie.NewVerkleTrie(verkle.New(), trie.NewDatabase(memorydb.New())).TryUpdate([]byte("00000000000000000000000000000012"), []byte("B"))

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				trie := trie.NewVerkleTrie(verkle.New(), trie.NewDatabase(memorydb.New()))
				for k := 0; k < len(accounts); k++ {
					trie.TryUpdateAccount(accounts[k].address[:], &accounts[k].stateAccount)
				}
				trie.Commit(true)
			}
		})
	}
}

type randomAccount struct {
	address      common.Address
	stateAccount types.StateAccount
}

func getRandomStateAccounts(rand *rand.Rand, count int) []randomAccount {
	randomBytes := func(size int) []byte {
		ret := make([]byte, size)
		rand.Read(ret)
		return ret
	}

	accounts := make([]randomAccount, count)
	for i := range accounts {
		accounts[i] = randomAccount{
			address: common.BytesToAddress(randomBytes(common.AddressLength)),
			stateAccount: types.StateAccount{
				Nonce:    rand.Uint64(),
				Balance:  big.NewInt(int64(rand.Uint64())),
				Root:     common.Hash{},
				CodeHash: nil,
			},
		}
	}
	return accounts
}

func BenchmarkTriesRandomVKTStateless(b *testing.B) {
	numAccounts := []int{1_000, 5_000, 10_000}
	state.TestVKTOpenStateless = true

	for _, numAccounts := range numAccounts {
		rs := rand.New(rand.NewSource(42))
		accounts := getRandomStateAccounts(rs, numAccounts)

		b.Run(fmt.Sprintf("%d accounts", numAccounts), func(b *testing.B) {
			trieDB := state.NewDatabaseWithConfig(rawdb.NewMemoryDatabase(), &trie.Config{UseVerkle: true})

			prevTrie, _ := trieDB.OpenTrie(common.Hash{})
			for k := 0; k < len(accounts); k++ {
				prevTrie.TryUpdateAccount(accounts[k].address[:], &accounts[k].stateAccount)
			}
			prevTrie.Commit(false)

			accounts := getRandomStateAccounts(rs, numAccounts)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				trie, err := trieDB.OpenTrie(prevTrie.Hash())
				if err != nil {
					b.Fatal(err)
				}
				for k := 0; k < len(accounts); k++ {
					trie.TryUpdateAccount(accounts[k].address[:], &accounts[k].stateAccount)
				}
				trie.Commit(true)
			}
		})
	}
}
