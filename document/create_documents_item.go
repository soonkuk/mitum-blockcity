package document

import (
	"github.com/spikeekips/mitum-currency/currency"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/hint"
	"github.com/spikeekips/mitum/util/isvalid"
)

var (
	CreateDocumentsItemImplType   = hint.Type("mitum-create-documents-item")
	CreateDocumentsItemImplHint   = hint.NewHint(CreateDocumentsItemImplType, "v0.0.1")
	CreateDocumentsItemImplHinter = CreateDocumentsItemImpl{BaseHinter: hint.NewBaseHinter(CreateDocumentsItemImplHint)}
)

type CreateDocumentsItemImpl struct {
	hint.BaseHinter
	doctype hint.Type
	doc     DocumentData
	cid     currency.CurrencyID
}

func NewCreateDocumentsItemImpl(
	doc DocumentData,
	cid currency.CurrencyID) CreateDocumentsItemImpl {

	if doc.Info().docType != doc.Hint().Type() {
		panic(util.WrongTypeError.Errorf("Document Info Type not matched with DocumentData Type, not %v", doc.Hint().Type()))
	}

	return CreateDocumentsItemImpl{
		BaseHinter: hint.NewBaseHinter(CreateDocumentsItemImplHint),
		doctype:    doc.Info().docType,
		doc:        doc,
		cid:        cid,
	}
}

func (it CreateDocumentsItemImpl) Bytes() []byte {
	bs := make([][]byte, 3)
	bs[0] = it.doctype.Bytes()
	bs[1] = it.doc.Bytes()
	bs[2] = it.cid.Bytes()

	return util.ConcatBytesSlice(bs...)
}

func (it CreateDocumentsItemImpl) IsValid([]byte) error {

	if err := isvalid.Check(
		nil, false,
		it.BaseHinter,
		it.doctype,
		it.doc,
		it.cid,
	); err != nil {
		return isvalid.InvalidError.Errorf("invalid CreateDocumentsItem: %w", err)
	}
	return nil
}

func (it CreateDocumentsItemImpl) DocumentId() string {
	return it.doc.DocumentId()
}

func (it CreateDocumentsItemImpl) DocType() hint.Type {
	return it.doctype
}

func (it CreateDocumentsItemImpl) Doc() DocumentData {
	return it.doc
}

func (it CreateDocumentsItemImpl) Currency() currency.CurrencyID {
	return it.cid
}

func (it CreateDocumentsItemImpl) Rebuild() CreateDocumentsItem {
	return it
}
