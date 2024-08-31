package DDON

import (
	"blockChain_consensus/tangleChain/common"
	"blockChain_consensus/tangleChain/crypto"
	"blockChain_consensus/tangleChain/database"
	loglogrus "blockChain_consensus/tangleChain/log_logrus"
	"blockChain_consensus/tangleChain/message"
	"blockChain_consensus/tangleChain/rlp"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"
)

const (
	K           int    = 2 // 分叉系数,默认是2
	defaultDiff uint64 = 3 // 默认的Pow难度值
)

// 表示交易类型,依据此来执行不同的合约函数
const (
	CommonWriteCode        = 0x00 // 执行 Common:Write 合约函数
	CommonReadCode         = 0x01 // 执行 Common:Read 合约函数
	CommonWriteAndReadCode = 0x02 // 同时执行 Common:Write 和 Common:Read 合约函数

	// TODO: BGP相关交易类型
)

// TODO:会选出一样的交易，这个bug需要修改
func RandomApproveStrategy(allTx []*RawTransaction) []*RawTransaction {
	var count int
	if len(allTx) < K {
		count = len(allTx)
	} else {
		count = K
	}

	approveTxs := make([]*RawTransaction, count)
	indexSet := make([]int, count)

	for i := 0; i < count; i++ {
		for {
			dup := false
			rand.Seed(time.Now().UnixNano())
			time.Sleep(time.Nanosecond)
			index := rand.Intn(count)

			for j := 0; j < i; j++ {
				if indexSet[j] == index {
					dup = true
					break
				}
			}

			if dup {
				continue
			} else {
				indexSet[i] = index
				break
			}
		}

	}

	for i := 0; i < count; i++ {
		approveTxs[i] = allTx[indexSet[i]]
	}

	return approveTxs
}

func TriconTipSelection(allTx []*RawTransaction, txDatabase database.Database, GenesisTxID common.Hash, seqID uint64) []*RawTransaction {
	// // var count int
	// if len(allTx) < K {
	// 	count = len(allTx)
	// } else {
	// 	count = K
	// }

	approveTxs := make([]*RawTransaction, 0)
	for i := 0; i < len(allTx); i++ {
		if allTx[i].SeqId == seqID {
			// loglogrus.Log.Infof("[Tangle] tips count(%x)\n", i)
			approveTxs = append(approveTxs, allTx[i])
			// fmt.Println(approveTxs)

		}
	}
	key := GenesisTxID[:]
	GenesisTxBytes, _ := txDatabase.Get(key)
	GenesisTx := TransactionDeSerialize(GenesisTxBytes)

	twoapproveTxs := twotipSelection(GenesisTx, txDatabase)
	// approveTxs = append(approveTxs, twoapproveTxs)
	fmt.Println("tttttttttttttttttttttttttttt11111")
	fmt.Println(twoapproveTxs.TxID)
	// fmt.Println(approveTxs)
	fmt.Println(len(approveTxs))

	return approveTxs
}

func twotipSelection(Tx *RawTransaction, txDatabase database.Database) *RawTransaction {
	//TODO：
	//子交易的所有cw放在列表，取最大 CWy
	//计算 exp(CWy - CWx) / exp(num)
	//
	if len(Tx.BeApprovedTx) == 0 {
		return Tx
	} else if len(Tx.BeApprovedTx) == 1 {
		key := Tx.BeApprovedTx[0][:]
		BeApprovedTxBytes, _ := txDatabase.Get(key)
		BeApprovedTx := TransactionDeSerialize(BeApprovedTxBytes)
		return twotipSelection(BeApprovedTx, txDatabase)
	} else {
		len := len(Tx.BeApprovedTx)
		rand.Seed(time.Now().UnixNano())
		time.Sleep(time.Nanosecond)
		index := rand.Intn(len)
		txSelect := Tx.BeApprovedTx[index]
		key := txSelect[:]
		BeApprovedTxBytes, _ := txDatabase.Get(key)
		BeApprovedTx := TransactionDeSerialize(BeApprovedTxBytes)
		return twotipSelection(BeApprovedTx, txDatabase)
	}

	// return approveTips
}

type RawTransaction struct {
	TxID   common.Hash
	TxCode uint64

	Args []string // 合约函数参数

	PreviousTxs []common.Hash // 前面的可供Approve的所有交易

	GenesisTx        common.Hash   // 创世交易哈希值
	Sender           common.NodeID // 交易的上传者
	Diff             uint64        // Pow难度值
	Nonce            uint64        // 随机值
	TimeStamp        uint64        // 交易产生的时间戳
	ApproveTx        []common.Hash // 支持的Previous交易
	BeApprovedTx     []common.Hash // 支持的后续交易
	IsGenesis        byte          // 是否是创世交易( == 1 表示是创世交易)
	Height           uint64        // 到Genesis的最大长度
	CumulativeWeight uint64        // 累积权重(当前到tip)
	Score            uint64        // 分数(当前到Genesis)

	// Depth和Weight都是会变化的,因此不适合参与到TxID的计算中
	Depth  uint64 // 到tip的最大长度
	Weight uint64 // 自身权重，默认是1

	MKindex uint64 //maker index
	SeqId   uint64 //maker sequence number

}

// graph结构
type Transaction struct {
	RawTx           *RawTransaction
	ApproveStrategy func([]*RawTransaction, database.Database, common.Hash, uint64) []*RawTransaction // ApproveTx选择策略
}

// 创建一个创世交易(默认所有节点都有，而且相同)
func NewGenesisTx(sender common.NodeID, powDiff uint64) *Transaction {
	rawTx := &RawTransaction{
		PreviousTxs:      make([]common.Hash, 0),
		ApproveTx:        make([]common.Hash, 0),
		BeApprovedTx:     make([]common.Hash, 0),
		Sender:           sender,
		Diff:             powDiff,
		IsGenesis:        1,
		Weight:           10,
		CumulativeWeight: 1,

		MKindex: 1000,
		SeqId:   1000,
	}
	tx := &Transaction{
		RawTx: rawTx,
	}
	tx.RawTx.TxID = tx.Hash() // 创世交易不需要进行Pow证明，直接计算哈希即可

	return tx
}

func NewTransaction(args []string, tipTx []common.Hash, sender common.NodeID, txCode uint64, powDiff uint64, MKindex uint64, seqId uint64, GenesisTx common.Hash) *Transaction {
	rawTx := &RawTransaction{
		TxCode: txCode,
		Args:   args,

		PreviousTxs: tipTx,
		Sender:      sender,
		Diff:        powDiff,
		IsGenesis:   0,
		Weight:      1,
		TimeStamp:   uint64(time.Now().UnixNano()),

		MKindex:          MKindex,
		SeqId:            seqId,
		GenesisTx:        GenesisTx,
		CumulativeWeight: 1,
	}

	tx := &Transaction{
		RawTx: rawTx,
		// ApproveStrategy: RandomApproveStrategy,
		ApproveStrategy: TriconTipSelection,
	}

	return tx
}

// 在所有tip中选择K个交易进行approve
func (tx *Transaction) SelectApproveTx(txDatabase database.Database) {
	if tx.RawTx.IsGenesis == 1 { // 当前交易是创世交易
		return
	}

	loglogrus.Log.Infof("[Tangle] len(tx.RawTx.PreviousTxs)(%x)\n", len(tx.RawTx.PreviousTxs))
	fmt.Println(tx.RawTx.PreviousTxs)
	// fmt.Println(tx.RawTx.GenesisTx)
	// fmt.Println(tx.RawTx.SeqId)

	if len(tx.RawTx.PreviousTxs) == 1 && reflect.DeepEqual(tx.RawTx.PreviousTxs[0], tx.RawTx.GenesisTx) { // 前面只有一个创世交易
		genesisHash := tx.RawTx.PreviousTxs[0]

		tx.RawTx.ApproveTx = append(tx.RawTx.ApproveTx, genesisHash) // 只能支持创世交易
		tx.RawTx.Height += 1
		tx.RawTx.Score += 1

		loglogrus.Log.Infof("[Tangle] 当前tip集合中只有创世交易(%x)\n", tx.RawTx.TxID)

		return
	}

	// 从数据库中查询当前交易的所有 PreviousTx
	previousRawTxs := make([]*RawTransaction, 0)
	for _, preTx := range tx.RawTx.PreviousTxs {
		key := preTx[:]
		txBytes, _ := txDatabase.Get(key)
		txEntity := TransactionDeSerialize(txBytes)
		previousRawTxs = append(previousRawTxs, txEntity)

		loglogrus.Log.Infof("[Tangle] 当前 PreviousTxs 集合 -- (index:) (txID:%x)\n", preTx)
		fmt.Println(txEntity.BeApprovedTx)
	}

	approveTxs := tx.ApproveStrategy(previousRawTxs, txDatabase, tx.RawTx.GenesisTx, tx.RawTx.SeqId) // 从所有的tip交易(tip交易必定是已经上链的,也就是一定是已经存储在数据库中的)中选出K个交易进行approve

	for _, approveTx := range approveTxs {
		tx.RawTx.ApproveTx = append(tx.RawTx.ApproveTx, approveTx.TxID)
		//loglogrus.Log.Infof("[Tangle] 当前tip集合 -- (index:%d) (txID:%x)\n", index, approveTx.TxID)
	}

	// 更改被选中为 approveTx 的 PreviousTx 的状态 和 当前交易的状态
	maxHeight := uint64(0)
	for _, approveTx := range approveTxs {
		tx.RawTx.Score += approveTx.Score // 当前交易的score要增加，加上所支持交易的score
		if approveTx.Height > maxHeight {
			maxHeight = approveTx.Height
		}
	}

	tx.RawTx.Height = maxHeight // 更新当前交易的height
}

// 在所有tip中选择K个交易进行approve
func (tx *Transaction) TriconTipSelectApproveTx(txDatabase database.Database, markerSet map[uint64]common.Hash) {
	if tx.RawTx.IsGenesis == 1 { // 当前交易是创世交易
		return
	}

	if tx.RawTx.MKindex == 1 {
		tx.RawTx.ApproveTx = append(tx.RawTx.ApproveTx, tx.RawTx.GenesisTx) // 只能支持创世交易
		tx.RawTx.Height += 1
		tx.RawTx.Score += 1

		loglogrus.Log.Infof("[Tangle] 当前tip集合中只有创世交易(%x)\n", tx.RawTx.TxID)

		return
	}
	tx.RawTx.ApproveTx = append(tx.RawTx.ApproveTx, markerSet[tx.RawTx.MKindex-1])
	tx.RawTx.Height += 1
	tx.RawTx.Score += 1
	// 更改被选中为 approveTx 的 PreviousTx 的状态 和 当前交易的状

	// tx.RawTx.Height = maxHeight // 更新当前交易的height
}

func (tx *Transaction) BackPreviousAndTipTxs(tips []common.Hash) []common.Hash {
	approvedTxs := tx.RawTx.ApproveTx

	result := make([]common.Hash, 0)
	for _, approvedTx := range approvedTxs {
		for _, tip := range tips {
			if reflect.DeepEqual(approvedTx, tip) {
				result = append(result, approvedTx)
			}
		}
	}
	return result
}

// 更新一笔新Tx(新Tx是指刚刚完成上链的交易,也就是刚刚成为tip的交易)之前的所有Tx  (此更新任务可以异步进行)
func (tx *Transaction) UpdatePreviousTx(txDatabase database.Database, markerSet map[uint64]common.Hash) {
	if len(tx.RawTx.PreviousTxs) == 1 && reflect.DeepEqual(tx.RawTx.PreviousTxs[0], tx.RawTx.GenesisTx) { // 前方只有一笔创始交易(这种情况简单,只需要更新创世交易信息)
		genesisHash := tx.RawTx.PreviousTxs[0]

		// 在数据库中查询到创世交易
		key := genesisHash[:]
		txBytes, _ := txDatabase.Get(key)
		genesisTx := TransactionDeSerialize(txBytes)

		// 更新创世交易的权重和深度
		// fmt.Println("222222222222222222222222222222weight")
		genesisTx.CumulativeWeight += tx.RawTx.Weight
		genesisTx.Depth += 1
		// fmt.Println(genesisTx.Weight)
		// fmt.Println(tx.RawTx.Weight)
		genesisTx.BeApprovedTx = append(genesisTx.BeApprovedTx, tx.RawTx.TxID)
		// 重新存入数据库
		value := TransactionSerialize(genesisTx)
		txDatabase.Put(key, value)
	} else { // 前方为普通情况,即有若干笔普通交易(这种情况较为复杂,需要迭代更新前方所有的交易)
		// previousRawTxs := make([]*RawTransaction, 0)

		if BeApprovedTxId, ok := markerSet[tx.RawTx.MKindex+1]; ok {
			tx.RawTx.BeApprovedTx = append(tx.RawTx.BeApprovedTx, BeApprovedTxId)
			fmt.Println("111111testtttttttttttttttttttttttttttttttttttt11235566")
			fmt.Println(BeApprovedTxId)
		}

		for _, preTx := range tx.RawTx.ApproveTx {
			key := preTx[:]
			txBytes, _ := txDatabase.Get(key)
			fmt.Println(key)
			fmt.Println(txBytes)
			if txBytes != nil {
				txEntity := TransactionDeSerialize(txBytes)
				// previousRawTxs = append(previousRawTxs, txEntity)
				txEntity.BeApprovedTx = append(txEntity.BeApprovedTx, tx.RawTx.TxID) // 更新前继交易的后续关系
				value := TransactionSerialize(txEntity)
				txDatabase.Put(key, value)
			}
		}

	}
}

func (tx *Transaction) updateCumulativeWeight(Tx *RawTransaction, txDatabase database.Database) {

	for _, preTx := range Tx.ApproveTx {
		// fmt.Println("开始递归aaaaaa")
		key := preTx[:]
		pretxBytes, _ := txDatabase.Get(key)
		pretxEntity := TransactionDeSerialize(pretxBytes)
		pretxEntity.CumulativeWeight += tx.RawTx.Weight // 累计权重更新
		pretxEntity.Depth += 1                          // 被支持交易的深度+1
		value := TransactionSerialize(pretxEntity)
		txDatabase.Put(key, value)
		fmt.Println(pretxEntity.MKindex)
		fmt.Println(pretxEntity.IsGenesis)
		if pretxEntity.IsGenesis == 1 {
			tx.updateCumulativeWeight(pretxEntity, txDatabase)
		}
	}

}

// 当前交易进行Pow证明（根据难度值计算nonce）
func (tx *Transaction) Pow() {
	targetPrefix := strings.Repeat("0", int(tx.RawTx.Diff))
	for {
		tx.RawTx.TxID = tx.Hash()
		if strings.HasPrefix(fmt.Sprintf("%x", tx.RawTx.TxID), targetPrefix) {
			return
		}
		tx.RawTx.Nonce++
	}
}

// 验证一笔交易是否合法(用于验证来自于其他节点广播的交易)
func (tx *Transaction) PowValidator() bool {
	targetPrefix := strings.Repeat("0", int(tx.RawTx.Diff))

	if strings.HasPrefix(fmt.Sprintf("%x", tx.RawTx.TxID), targetPrefix) {
		//loglogrus.Log.Infof("[Tangle] tx(%x) 完成Pow验证\n", tx.RawTx.TxID)
		return true
	} else {
		return false
	}
}

// 求交易的哈希值
func (tx *Transaction) Hash() common.Hash {
	target := tx.RawTx
	// target.Depth = 0
	// target.Weight = 0

	summary, err := rlp.EncodeToBytes(target)
	if err != nil {
		loglogrus.Log.Warnf("[Tangle] 计算交易哈希值失败,err:%v\n", err)
		return common.Hash{}
	}
	tx.RawTx.TxID = crypto.Sha3Hash(summary)
	return tx.RawTx.TxID
}

// 简单的实现合约功能(写入操作)
func (tx *Transaction) CommonExecuteWrite(worldState database.Database, key, value string) {
	keyBytes := []byte(key)
	valueBytes := []byte(value)

	worldState.Put(keyBytes, valueBytes)
}

// 简单的实现合约功能(读取操作)
func (tx *Transaction) CommonExecuteRead(worldState database.Database, key string) string {
	keyBytes := []byte(key)
	valueBytes, _ := worldState.Get(keyBytes)

	return string(valueBytes)
}

func TransactionSerialize(rawTx *RawTransaction) []byte {
	enc, _ := rlp.EncodeToBytes(rawTx)
	return enc
}

func TransactionDeSerialize(byteStream []byte) *RawTransaction {
	rawTx := new(RawTransaction)
	if err := rlp.DecodeBytes(byteStream, rawTx); err != nil {
		loglogrus.Log.Warnf("[Tangle] 无法从rlp字节流中解析出RawTransaction,err:%v\n", err)
		return nil
	} else {
		return rawTx
	}
}

// 将交易编辑成Wrap Message
func EncodeTxToWrapMsg(tx *Transaction, prv *ecdsa.PrivateKey) *message.WrapMessage {

	if payload, err := json.Marshal(tx.RawTx); err != nil {
		loglogrus.Log.Warnf("[Tangle] 无法将Transaction编辑为json字节流,err=%v\n", err)
		return nil
	} else {
		cMsg := message.NewCommonMsg(payload, tx.RawTx.Sender, prv)
		wrapMsg := message.EncodeToWrapMessage(cMsg)
		return wrapMsg
	}

}

// 从Wrap Message中解包出交易
func DecodeTxFromCommonMsg(wrapMsg *message.WrapMessage) *RawTransaction {
	cMsg := message.DecodeWrapMessage(wrapMsg)
	txBytes := cMsg.BackPayload().(string)

	tx := new(RawTransaction)
	if err := json.Unmarshal([]byte(txBytes), tx); err != nil {
		loglogrus.Log.Warnf("[Tangle] 无法将Payload字节流解析为Transaction,err=%v\n", err)
		return nil
	} else {
		return tx
	}
}

func DecodeTxFromJsonStr(str string) *Transaction {
	rawTx := new(RawTransaction)
	if err := json.Unmarshal([]byte(str), rawTx); err != nil {
		loglogrus.Log.Warnf("[Tangle] 无法将Payload字节流解析为Transaction,err=%v\n", err)
		return nil
	} else {
		tx := new(Transaction)
		tx.RawTx = rawTx
		return tx
	}
}
