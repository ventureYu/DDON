package main

import (
	"blockChain_consensus/tangleChain/DDON"
	loglogrus "blockChain_consensus/tangleChain/log_logrus"
	"blockChain_consensus/tangleChain/p2p"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	peerUrl    string
	peersTable string

	sendRate          int // 节点发送交易的速率
	txConfirmDuration int // 交易从发布到上链所需的确定时延

	powDiff uint64 // pow难度
	seqId   uint64 // marker sequence number
	MKindex uint64 // marker index number
)

func init() {
	flag.StringVar(&peerUrl, "peerUrl", "127.0.0.1:65588", "DDON节点的Url监听地址")
	flag.StringVar(&peersTable, "peersTable", "127.0.0.1:8001,127.0.0.1:8002,127.0.0.1:8003", "DDON集群中所有节点的Url地址")

	flag.IntVar(&sendRate, "sendRate", 1, "节点发送交易的速率")
	flag.IntVar(&txConfirmDuration, "txCD", 4, "交易从发布到上链所需的确定时延")

	flag.Uint64Var(&powDiff, "powDiff", 3, "节点生成交易时的pow难度")
	flag.Uint64Var(&seqId, "seqId", 1, "Marker序列号")
}

func main() {
	flag.Parse()

	otherPeers := make([]string, 0)

	otherPeers = strings.Split(peersTable, ",")

	peer := p2p.NewPeer(peerUrl, otherPeers) // 建立本地p2p节点

	DDONPeer := DDON.NewDDON(sendRate, time.Duration(txConfirmDuration)*time.Second, powDiff, peer) // 在p2p节点之上创建DDON节点

	ctx, finFunc := context.WithCancel(context.Background())

	DDONPeer.Start(ctx) // 启动DDON节点

	txSendCycle := time.NewTicker(2 * time.Second)
	finTimer := time.NewTimer(8 * time.Second)

	startTime := time.Now()
	MKindex = 1
	for {
		select {
		case <-txSendCycle.C:
			for i := 0; i < sendRate; i++ {
				DDONPeer.PublishTransaction(DDON.CommonWriteAndReadCode, []string{fmt.Sprintf("test_key%d", i), fmt.Sprintf("test_value%d", i)}, MKindex, seqId)
				MKindex++
			}
		case <-finTimer.C:
			txSendCycle.Stop()
			finFunc()

			txCount := DDONPeer.BackTxCount()
			endTime := time.Now()
			loglogrus.Log.Infof(" 当前节点(%s) -- 起始时间(%s) -- 终止时间(%s) -- 时间差(%v) -- 上链交易数为:%d\n", peerUrl,
				startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"), endTime.Sub(startTime).Seconds(), txCount)

			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
	}

}
