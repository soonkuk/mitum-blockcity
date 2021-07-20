package blocksign

import (
	"github.com/soonkuk/mitum-data/currency"
	"github.com/spikeekips/mitum/base"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
)

type TransferDocumentsItemJSONPacker struct {
	jsonenc.HintedHead
	DM DocId               `json:"document"`
	OW base.Address        `json:"owwner"`
	RC base.Address        `json:"receiver"`
	CI currency.CurrencyID `json:"currency"`
}

func (it BaseTransferDocumentsItem) MarshalJSON() ([]byte, error) {
	return jsonenc.Marshal(TransferDocumentsItemJSONPacker{
		HintedHead: jsonenc.NewHintedHead(it.Hint()),
		DM:         it.documentId,
		OW:         it.owner,
		RC:         it.receiver,
		CI:         it.cid,
	})
}

type BaseTransferDocumentsItemJSONUnpacker struct {
	DM []byte              `json:"document"`
	OW base.AddressDecoder `json:"owner"`
	RC base.AddressDecoder `json:"receiver"`
	CI string              `json:"currency"`
}

func (it *BaseTransferDocumentsItem) UnpackJSON(b []byte, enc *jsonenc.Encoder) error {
	var ht jsonenc.HintedHead
	if err := enc.Unmarshal(b, &ht); err != nil {
		return err
	}

	var uit BaseTransferDocumentsItemJSONUnpacker
	if err := enc.Unmarshal(b, &uit); err != nil {
		return err
	}

	return it.unpack(enc, ht.H, uit.DM, uit.OW, uit.RC, uit.CI)
}