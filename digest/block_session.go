package digest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/pkg/errors"
	"github.com/soonkuk/mitum-blocksign/document"
	"github.com/spikeekips/mitum-currency/currency"
	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/base/operation"
	"github.com/spikeekips/mitum/base/state"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/tree"
	"github.com/spikeekips/mitum/util/valuehash"
)

var bulkWriteLimit = 500

type BlockSession struct {
	sync.RWMutex
	block           block.Block
	st              *Database
	opsTreeNodes    map[string]operation.FixedTreeNode
	operationModels []mongo.WriteModel
	accountModels   []mongo.WriteModel
	//bsDocumentModels  []mongo.WriteModel
	documentModels []mongo.WriteModel
	//bsDocumentsModels []mongo.WriteModel
	documentsModels []mongo.WriteModel
	balanceModels   []mongo.WriteModel
	statesValue     *sync.Map
	// bsDocumentList    []currency.Big
	documentList []string
}

func NewBlockSession(st *Database, blk block.Block) (*BlockSession, error) {
	if st.Readonly() {
		return nil, errors.Errorf("readonly mode")
	}

	nst, err := st.New()
	if err != nil {
		return nil, err
	}

	return &BlockSession{
		st:          nst,
		block:       blk,
		statesValue: &sync.Map{},
	}, nil
}

func (bs *BlockSession) Prepare() error {
	bs.Lock()
	defer bs.Unlock()

	if err := bs.prepareOperationsTree(); err != nil {
		return err
	}

	if err := bs.prepareOperations(); err != nil {
		return err
	}

	return bs.prepareAccounts()
}

func (bs *BlockSession) Commit(ctx context.Context) error {
	bs.Lock()
	defer bs.Unlock()

	started := time.Now()
	defer func() {
		bs.statesValue.Store("commit", time.Since(started))

		_ = bs.close()
	}()

	if err := bs.st.CleanByHeight(bs.block.Height()); err != nil {
		return err
	}

	if err := bs.writeModels(ctx, defaultColNameOperation, bs.operationModels); err != nil {
		return err
	}

	if err := bs.writeModels(ctx, defaultColNameAccount, bs.accountModels); err != nil {
		return err
	}

	if err := bs.writeModels(ctx, defaultColNameBalance, bs.balanceModels); err != nil {
		return err
	}

	if len(bs.documentModels) > 0 {

		for i := range bs.documentList {
			if err := bs.st.cleanByHeightColNameDocumentId(bs.block.Height(), defaultColNameDocument, bs.documentList[i]); err != nil {
				return err
			}
		}

		if err := bs.writeModels(ctx, defaultColNameDocument, bs.documentModels); err != nil {
			return err
		}
	}

	if len(bs.documentsModels) > 0 {
		if err := bs.writeModels(ctx, defaultColNameDocuments, bs.documentsModels); err != nil {
			return err
		}
	}

	return nil
}

func (bs *BlockSession) Close() error {
	bs.Lock()
	defer bs.Unlock()

	return bs.close()
}

func (bs *BlockSession) prepareOperationsTree() error {
	nodes := map[string]operation.FixedTreeNode{}
	if err := bs.block.OperationsTree().Traverse(func(no tree.FixedTreeNode) (bool, error) {
		nno := no.(operation.FixedTreeNode)
		fh := valuehash.NewBytes(nno.Key())
		nodes[fh.String()] = nno

		return true, nil
	}); err != nil {
		return err
	}

	bs.opsTreeNodes = nodes

	return nil
}

func (bs *BlockSession) prepareOperations() error {
	if len(bs.block.Operations()) < 1 {
		return nil
	}

	node := func(h valuehash.Hash) (bool /* found */, bool /* instate */, operation.ReasonError) {
		no, found := bs.opsTreeNodes[h.String()]
		if !found {
			return false, false, nil
		}

		return true, no.InState(), no.Reason()
	}

	bs.operationModels = make([]mongo.WriteModel, len(bs.block.Operations()))

	for i := range bs.block.Operations() {
		op := bs.block.Operations()[i]

		found, inState, reason := node(op.Fact().Hash())
		if !found {
			return util.NotFoundError.Errorf("operation, %s not found in operations tree", op.Fact().Hash().String())
		}

		doc, err := NewOperationDoc(
			op,
			bs.st.database.Encoder(),
			bs.block.Height(),
			bs.block.ConfirmedAt(),
			inState,
			reason,
			uint64(i),
		)
		if err != nil {
			return err
		}
		bs.operationModels[i] = mongo.NewInsertOneModel().SetDocument(doc)
	}

	return nil
}

func (bs *BlockSession) prepareAccounts() error {
	if len(bs.block.States()) < 1 {
		return nil
	}

	var accountModels []mongo.WriteModel
	var balanceModels []mongo.WriteModel
	// var bsDocumentModels []mongo.WriteModel
	// var bsDocumentsModels []mongo.WriteModel
	var documentModels []mongo.WriteModel
	var documentsModels []mongo.WriteModel

	for i := range bs.block.States() {
		st := bs.block.States()[i]
		switch {
		case currency.IsStateAccountKey(st.Key()):
			j, err := bs.handleAccountState(st)
			if err != nil {
				return err
			}
			accountModels = append(accountModels, j...)
		case currency.IsStateBalanceKey(st.Key()):
			j, err := bs.handleBalanceState(st)
			if err != nil {
				return err
			}
			balanceModels = append(balanceModels, j...)
		case document.IsStateDocumentDataKey(st.Key()):
			if j, err := bs.handleDocumentDataState(st); err != nil {
				return err
			} else {
				documentModels = append(documentModels, j...)
			}
		case document.IsStateDocumentsKey(st.Key()):
			if j, err := bs.handleDocumentsState(st); err != nil {
				return err
			} else {
				documentsModels = append(documentsModels, j...)
			}
		default:
			continue
		}
	}

	bs.accountModels = accountModels
	bs.balanceModels = balanceModels

	if len(documentModels) > 0 {
		bs.documentModels = documentModels
	}

	if len(documentsModels) > 0 {
		bs.documentsModels = documentsModels
	}

	return nil
}

func (bs *BlockSession) handleAccountState(st state.State) ([]mongo.WriteModel, error) {
	if rs, err := NewAccountValue(st); err != nil {
		return nil, err
	} else if doc, err := NewAccountDoc(rs, bs.st.database.Encoder()); err != nil {
		return nil, err
	} else {
		return []mongo.WriteModel{mongo.NewInsertOneModel().SetDocument(doc)}, nil
	}
}

func (bs *BlockSession) handleBalanceState(st state.State) ([]mongo.WriteModel, error) {
	doc, err := NewBalanceDoc(st, bs.st.database.Encoder())
	if err != nil {
		return nil, err
	}
	return []mongo.WriteModel{mongo.NewInsertOneModel().SetDocument(doc)}, nil
}

func (bs *BlockSession) handleDocumentDataState(st state.State) ([]mongo.WriteModel, error) {
	doc, err := document.StateDocumentDataValue(st)
	if err != nil {
		return nil, err
	}
	if ndoc, err := NewDocumentDoc(bs.st.database.Encoder(), doc, bs.block.Height()); err != nil {
		return nil, err
	} else {
		bs.documentList = append(bs.documentList, ndoc.DocumentId())
		return []mongo.WriteModel{mongo.NewInsertOneModel().SetDocument(ndoc)}, nil
	}
}

func (bs *BlockSession) handleDocumentsState(st state.State) ([]mongo.WriteModel, error) {
	if doc, err := NewDocumentsDoc(st, bs.st.database.Encoder()); err != nil {
		return nil, err
	} else {
		return []mongo.WriteModel{mongo.NewInsertOneModel().SetDocument(doc)}, nil
	}
}

func (bs *BlockSession) writeModels(ctx context.Context, col string, models []mongo.WriteModel) error {
	started := time.Now()
	defer func() {
		bs.statesValue.Store(fmt.Sprintf("write-models-%s", col), time.Since(started))
	}()

	n := len(models)
	if n < 1 {
		return nil
	} else if n <= bulkWriteLimit {
		return bs.writeModelsChunk(ctx, col, models)
	}

	z := n / bulkWriteLimit
	if n%bulkWriteLimit != 0 {
		z++
	}

	for i := 0; i < z; i++ {
		s := i * bulkWriteLimit
		e := s + bulkWriteLimit
		if e > n {
			e = n
		}

		if err := bs.writeModelsChunk(ctx, col, models[s:e]); err != nil {
			return err
		}
	}

	return nil
}

func (bs *BlockSession) writeModelsChunk(ctx context.Context, col string, models []mongo.WriteModel) error {
	opts := options.BulkWrite().SetOrdered(false)
	if res, err := bs.st.database.Client().Collection(col).BulkWrite(ctx, models, opts); err != nil {
		return storage.MergeStorageError(err)
	} else if res != nil && res.InsertedCount < 1 {
		return errors.Errorf("not inserted to %s", col)
	}

	return nil
}

func (bs *BlockSession) close() error {
	bs.block = nil
	bs.operationModels = nil
	bs.accountModels = nil
	bs.balanceModels = nil
	bs.documentModels = nil
	bs.documentsModels = nil

	return bs.st.Close()
}
