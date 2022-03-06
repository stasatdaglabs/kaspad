package mine

import (
	"fmt"
	"github.com/kaspanet/kaspad/stability-tests/common/copy"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/kaspanet/kaspad/domain/consensus"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/model/testapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
	"github.com/kaspanet/kaspad/domain/consensus/utils/mining"
	"github.com/pkg/errors"
)

func fromFile(jsonFile string, consensusConfig *consensus.Config, dataDir string) error {
	log.Infof("Mining blocks from JSON file %s from data directory %s", jsonFile, dataDir)
	blockChan, err := readBlocks(jsonFile)
	if err != nil {
		return err
	}

	return mineBlocks(consensusConfig, blockChan, dataDir)
}

func mineBlocks(consensusConfig *consensus.Config, blockChan <-chan JSONBlock, dataDir string) error {
	mdb, err := newMiningDB(dataDir)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(dataDir, consensusConfig.Name, "datadir2")
	factory := consensus.NewFactory()
	factory.SetTestDataDir(dbPath)
	testConsensus, tearDownFunc, err := factory.NewTestConsensus(consensusConfig, "minejson")
	if err != nil {
		return err
	}
	defer tearDownFunc(true)

	info, err := testConsensus.GetSyncInfo()
	if err != nil {
		return err
	}

	log.Infof("Starting with data directory with %d headers and %d blocks", info.HeaderCount, info.BlockCount)

	err = mdb.putID("0", consensusConfig.GenesisHash)
	if err != nil {
		return err
	}

	totalBlocksSubmitted := 0
	lastLogTime := time.Now()
	for blockData := range blockChan {
		block, err := mineOrFetchBlock(blockData, mdb, testConsensus)
		if err != nil {
			return err
		}

		totalBlocksSubmitted++
		const logInterval = 1000
		if totalBlocksSubmitted%logInterval == 0 {
			intervalDuration := time.Since(lastLogTime)
			blocksPerSecond := logInterval / intervalDuration.Seconds()
			log.Infof("It took %s to submit %d blocks (%f blocks/sec)"+
				" (total blocks sent %d)", intervalDuration, logInterval, blocksPerSecond, totalBlocksSubmitted)
			lastLogTime = time.Now()
		}

		blockHash := consensushashing.BlockHash(block)
		log.Tracef("Submitted block %s with hash %s", blockData.ID, blockHash)
	}
	return nil
}

func mineOrFetchBlock(blockData JSONBlock, mdb *miningDB, testConsensus testapi.TestConsensus) (*externalapi.DomainBlock, error) {
	hash := mdb.hashByID(blockData.ID)
	if mdb.hashByID(blockData.ID) != nil {
		return testConsensus.GetBlock(hash)
	}

	parentHashes := make([]*externalapi.DomainHash, len(blockData.Parents))
	for i, parentID := range blockData.Parents {
		parentHashes[i] = mdb.hashByID(parentID)
	}
	block, _, err := testConsensus.BuildBlockWithParents(parentHashes,
		&externalapi.DomainCoinbaseData{ScriptPublicKey: &externalapi.ScriptPublicKey{}}, []*externalapi.DomainTransaction{})
	if err != nil {
		return nil, errors.Wrap(err, "error in BuildBlockWithParents")
	}

	if !testConsensus.DAGParams().SkipProofOfWork {
		SolveBlock(block)
	}

	_, err = testConsensus.ValidateAndInsertBlock(block, true)
	if err != nil {
		return nil, errors.Wrap(err, "error in ValidateAndInsertBlock")
	}

	blockHash := consensushashing.BlockHash(block)
	err = mdb.putID(blockData.ID, blockHash)
	if err != nil {
		return nil, err
	}

	err = mdb.updateLastMinedBlock(blockData.ID)
	if err != nil {
		return nil, err
	}

	return block, nil
}

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

// SolveBlock increments the given block's nonce until it matches the difficulty requirements in its bits field
func SolveBlock(block *externalapi.DomainBlock) {
	mining.SolveBlock(block, random)
}

func PrepareSyncerDataDir(jsonFile string, consensusConfig *consensus.Config, dataDir string) error {
	tempDir := os.TempDir()
	_, file := path.Split(jsonFile)
	tempDataDirKey := fmt.Sprintf("%s-%s", file, consensusConfig.Name)
	tempDataDirPath := path.Join(tempDir, tempDataDirKey)

	_, err := os.Stat(tempDataDirPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err := fromFile(jsonFile, consensusConfig, tempDataDirPath)
		if err != nil {
			return err
		}
	}

	_, err = os.Stat(dataDir)
	if err == nil {
		err := os.RemoveAll(dataDir)
		if err != nil {
			return err
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return copy.CopyDirectory(tempDataDirPath, dataDir)
}
