package blockprocessor

import (
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/ruleerrors"
	"github.com/kaspanet/kaspad/domain/consensus/utils/hashserialization"
	"github.com/pkg/errors"
)

func (bp *blockProcessor) validateAndInsertBlock(block *externalapi.DomainBlock, headerOnly bool) error {
	hash := hashserialization.HeaderHash(block.Header)
	if headerOnly && len(block.Transactions) != 0 {
		return errors.Errorf("block %s contains transactions while validating in header only mode", hash)
	}

	err := bp.checkBlockStatus(hash, headerOnly)
	if err != nil {
		return err
	}

	err = bp.validateBlock(block, headerOnly)
	if err != nil {
		bp.discardAllChanges()
		return err
	}

	hasHeader, err := bp.hasHeader(hash)
	if err != nil {
		return err
	}

	if !hasHeader {
		err = bp.reachabilityManager.AddBlock(hash)
		if err != nil {
			return err
		}

		err = bp.headerTipsManager.AddHeaderTip(hash)
		if err != nil {
			return err
		}
	}

	if headerOnly {
		bp.blockStatusStore.Stage(hash, externalapi.StatusHeaderOnly)
	} else {
		bp.blockStatusStore.Stage(hash, externalapi.StatusUTXOPendingVerification)
	}

	// Block validations passed, save whatever DAG data was
	// collected so far
	err = bp.commitAllChanges()
	if err != nil {
		return err
	}

	if headerOnly {
		return nil
	}

	// Attempt to add the block to the virtual
	err = bp.consensusStateManager.AddBlockToVirtual(hash)
	if err != nil {
		return err
	}

	return bp.commitAllChanges()
}

func (bp *blockProcessor) checkBlockStatus(hash *externalapi.DomainHash, headerOnly bool) error {
	exists, err := bp.blockStatusStore.Exists(bp.databaseContext, hash)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	status, err := bp.blockStatusStore.Get(bp.databaseContext, hash)
	if err != nil {
		return err
	}

	if status == externalapi.StatusInvalid {
		return errors.Wrapf(ruleerrors.ErrKnownInvalid, "block %s is a known invalid block", hash)
	}

	if headerOnly || status != externalapi.StatusHeaderOnly {
		return errors.Wrapf(ruleerrors.ErrDuplicateBlock, "block %s already exists", hash)
	}

	return nil
}

func (bp *blockProcessor) validateBlock(block *externalapi.DomainBlock, headerOnly bool) error {
	// If any validation until (included) proof-of-work fails, simply
	// return an error without writing anything in the database.
	// This is to prevent spamming attacks.
	err := bp.validatePreProofOfWork(block)
	if err != nil {
		return err
	}

	blockHash := hashserialization.HeaderHash(block.Header)
	err = bp.blockValidator.ValidateProofOfWorkAndDifficulty(blockHash)
	if err != nil {
		return err
	}

	// If in-context validations fail, discard all changes and store the
	// block with StatusInvalid.
	err = bp.validatePostProofOfWork(block, headerOnly)
	if err != nil {
		if errors.As(err, &ruleerrors.RuleError{}) {
			bp.discardAllChanges()
			hash := hashserialization.HeaderHash(block.Header)
			bp.blockStatusStore.Stage(hash, externalapi.StatusInvalid)
			commitErr := bp.commitAllChanges()
			if commitErr != nil {
				return commitErr
			}
		}
		return err
	}
	return nil
}

func (bp *blockProcessor) validatePreProofOfWork(block *externalapi.DomainBlock) error {
	blockHash := hashserialization.HeaderHash(block.Header)

	hasHeader, err := bp.hasHeader(blockHash)
	if err != nil {
		return err
	}

	if hasHeader {
		return nil
	}

	err = bp.blockValidator.ValidateHeaderInIsolation(blockHash)
	if err != nil {
		return err
	}
	return nil
}

func (bp *blockProcessor) validatePostProofOfWork(block *externalapi.DomainBlock, headerOnly bool) error {
	blockHash := hashserialization.HeaderHash(block.Header)

	if !headerOnly {
		bp.blockStore.Stage(blockHash, block)
		err := bp.blockValidator.ValidateBodyInIsolation(blockHash)
		if err != nil {
			return err
		}
	}

	hasHeader, err := bp.hasHeader(blockHash)
	if err != nil {
		return err
	}

	if !hasHeader {
		bp.blockHeaderStore.Stage(blockHash, block.Header)

		err := bp.dagTopologyManager.SetParents(blockHash, block.Header.ParentHashes)
		if err != nil {
			return err
		}
		err = bp.blockValidator.ValidateHeaderInContext(blockHash)
		if err != nil {
			return err
		}
	}

	return nil
}

func (bp *blockProcessor) hasHeader(blockHash *externalapi.DomainHash) (bool, error) {
	exists, err := bp.blockStatusStore.Exists(bp.databaseContext, blockHash)
	if err != nil {
		return false, err
	}

	if !exists {
		return false, nil
	}

	status, err := bp.blockStatusStore.Get(bp.databaseContext, blockHash)
	if err != nil {
		return false, err
	}

	return status == externalapi.StatusHeaderOnly, nil
}

func (bp *blockProcessor) discardAllChanges() {
	for _, store := range bp.stores {
		store.Discard()
	}
}

func (bp *blockProcessor) commitAllChanges() error {
	dbTx, err := bp.databaseContext.Begin()
	if err != nil {
		return err
	}

	for _, store := range bp.stores {
		err = store.Commit(dbTx)
		if err != nil {
			return err
		}
	}

	return dbTx.Commit()
}