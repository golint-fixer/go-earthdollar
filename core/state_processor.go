package core

import (
	"math/big"
	
	"github.com/Earthdollar/go-earthdollar/core/state"
	"github.com/Earthdollar/go-earthdollar/core/types"
	"github.com/Earthdollar/go-earthdollar/core/vm"
	"github.com/Earthdollar/go-earthdollar/crypto"
	"github.com/Earthdollar/go-earthdollar/logger"
	"github.com/Earthdollar/go-earthdollar/logger/glog"
)

var (
	big8  = big.NewInt(8)
	big32 = big.NewInt(32)
)

type StateProcessor struct {
	bc *BlockChain
}

func NewStateProcessor(bc *BlockChain) *StateProcessor {
	return &StateProcessor{bc}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB) (types.Receipts, vm.Logs, *big.Int, error) {
	var (
		receipts     types.Receipts
		totalUsedGas = big.NewInt(0)
		err          error
		header       = block.Header()
		allLogs      vm.Logs
		gp           = new(GasPool).AddGas(block.GasLimit())
	)

	for i, tx := range block.Transactions() {
		statedb.StartRecord(tx.Hash(), block.Hash(), i)
		receipt, logs, _, err := ApplyTransaction(p.bc, gp, statedb, header, tx, totalUsedGas)
		if err != nil {
			return nil, nil, totalUsedGas, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, logs...)
	}

	//earthdollar
	rewards := AccumulateRewards(statedb, header, block.Uncles())
	PayRewards(statedb, header, block.Uncles(), rewards)

	return receipts, allLogs, totalUsedGas, err
}

// ApplyTransaction attemps to apply a transaction to the given state database
// and uses the input parameters for its environment.
//
// ApplyTransactions returns the generated receipts and vm logs during the
// execution of the state transition phase.
func ApplyTransaction(bc *BlockChain, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *big.Int) (*types.Receipt, vm.Logs, *big.Int, error) {
	_, gas, err := ApplyMessage(NewEnv(statedb, bc, tx, header), tx, gp)
	if err != nil {
		return nil, nil, nil, err
	}

	// Update the state with pending changes
	usedGas.Add(usedGas, gas)
	receipt := types.NewReceipt(statedb.IntermediateRoot().Bytes(), usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = new(big.Int).Set(gas)
	if MessageCreatesContract(tx) {
		from, _ := tx.From()
		receipt.ContractAddress = crypto.CreateAddress(from, tx.Nonce())
	}

	logs := statedb.GetLogs(tx.Hash())
	receipt.Logs = logs
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	glog.V(logger.Debug).Infoln(receipt)

	return receipt, logs, gas, err
} 

// AccumulateRewards credits the coinbase of the given block with the
// mining reward. The total reward consists of the static block reward
// and rewards for included uncles. The coinbase of each uncle block is
// also rewarded.
func AccumulateRewards(statedb *state.StateDB, header *types.Header, uncles []*types.Header) []*big.Int {
	miner_reward := new(big.Int).Set(BlockReward)
	r := new(big.Int)
	rewards := []*big.Int {}
	for _, uncle := range uncles {
		r.Add(uncle.Number, big8)
		r.Sub(r, header.Number)
		r.Mul(r, BlockReward)
		r.Div(r, big8)
		//statedb.AddBalance(uncle.Coinbase, r)
		rewards = append(rewards,r)
		
		r.Div(BlockReward, big32)
		miner_reward.Add(miner_reward, r)
	}
	//statedb.AddBalance(header.Coinbase, miner_reward)
	rewards = append(rewards, miner_reward)
	return rewards
}

//earthdollar
func PayRewards(statedb *state.StateDB, header *types.Header, uncles []*types.Header, rewards []*big.Int) {
	i := 0
	for _, uncle := range uncles {
		statedb.AddBalance(uncle.Coinbase, rewards[i])
		i++
	}
	statedb.AddBalance(header.Coinbase, rewards[i])
}

