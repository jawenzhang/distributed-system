package raftkv

import (
	"encoding/gob"
	"fmt"
	"github.com/astaxie/beego/logs"
	"labrpc"
	"log"
	"raft"
	"sync"
	"time"
)

const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

type KvMessage struct {
	Index int
	value chan string
}

//type OpType int
//
//const (
//	OpPut OpType = 0
//	OpAppend OpType = 1
//	OpGet OpType = 2
//)
//
//var OpTypeName = map[OpType]string{
//	OpPut:"OpPut",
//	OpAppend:"OpAppend",
//	OpGet:"OpGet",
//}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Type  string
	Key   string
	Value string
	Id    int64
	ReqId int64
}

type RaftKV struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	stopc     chan struct{}
	db        map[string]string
	result    map[int]chan Op
	ack       map[int64]int64
	prevIndex int
	//result map[int]string
	logger *logs.BeeLogger
}

const AppendTimeOut = 1000 * time.Millisecond

func (kv *RaftKV) AppendLog(entry Op) bool {
	index, _, isLeader := kv.rf.Start(entry)
	if !isLeader {
		return false
	}
	kv.mu.Lock()
	ch, ok := kv.result[index]
	if !ok {
		ch = make(chan Op, 1)
		kv.result[index] = ch
	}
	kv.mu.Unlock()
	kv.logger.Debug("wait leader:%d index:%d", kv.me, index)
	// TODO:优化超时的逻辑
	select {
	case op := <-ch:
		commited := op == entry
		kv.logger.Debug("index:%d commited:%v", index, commited)
		return commited
		// 此处超时其实也很好理解，因为刚开始是leader，但是在log得到commit之前，丢失了leadership，此时
		// 如果没有超时机制，则会一直阻塞下去
		// 或者由于此时的leader是一个分区里面的leader，则只可能一直阻塞下去了
		// 因此也需要超时
	case <-time.After(AppendTimeOut):
		kv.logger.Debug("index:%d %s timeout after %v", index, entry.Type, AppendTimeOut)
		return false
	}
}

func (kv *RaftKV) Get(args *GetArgs, reply *GetReply) {
	entry := Op{Type: "Get", Key: args.Key, Id: args.Id, ReqId: args.ReqId}
	ok := kv.AppendLog(entry)
	if !ok {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
		reply.Err = OK
		kv.mu.Lock()
		reply.Value = kv.db[args.Key]
		kv.ack[args.Id] = args.ReqId
		//log.Printf("%d get:%v value:%s\n",kv.me,entry,reply.Value)
		kv.mu.Unlock()
	}
}

func (kv *RaftKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	entry := Op{Type: args.Op, Key: args.Key, Value: args.Value, Id: args.Id, ReqId: args.ReqId}
	ok := kv.AppendLog(entry)
	if !ok {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
		reply.Err = OK
	}
}

//
// the tester calls Kill() when a RaftKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *RaftKV) Kill() {
	kv.rf.Kill()
	close(kv.stopc)
}

func (kv *RaftKV) checkDup(op *Op) bool {
	if v, ok := kv.ack[op.Id]; ok {
		return v >= op.ReqId
	} else {
		return false
	}
}

func (kv *RaftKV) getApply() {
	ticker := time.NewTicker(1000 * time.Millisecond) // 100ms
	defer ticker.Stop()                               // 定时发生器

	for {
		select {
		case applyMsg := <-kv.applyCh:
			index := applyMsg.Index
			op := applyMsg.Command.(Op)
			if index > 1 && kv.prevIndex != index-1 {
				// 判断之前的值是否处理过了
				kv.logger.Error("server %v apply out of order %v", kv.me, index)
				panic(fmt.Sprintf("unordered index:%d prevIndex:%d", index, kv.prevIndex))
			}
			kv.prevIndex = index
			kv.logger.Debug("index:%d Op:%s", index, op.Type)

			kv.mu.Lock()

			if !kv.checkDup(&op) {
				// 操作
				switch op.Type {
				case "Put":
					kv.db[op.Key] = op.Value
				case "Append":
					kv.db[op.Key] += op.Value
				}
				kv.ack[op.Id] = op.ReqId
			}
			// 通知结果
			ch, ok := kv.result[index]
			if ok {
				select {
				case <-ch:
				default:
				}
			} else {
				// 没人读就有了数据？
				ch = make(chan Op, 1)
				kv.result[index] = ch
			}
			ch <- op

			kv.mu.Unlock()
		case <-kv.stopc:
			return
		}
	}
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots with persister.SaveSnapshot(),
// and Raft should save its state (including log) with persister.SaveRaftState().
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *RaftKV {
	// call gob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	gob.Register(Op{})

	kv := new(RaftKV)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// Your initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	kv.logger = logs.NewLogger()
	kv.logger.SetLogger(logs.AdapterConsole)
	//rf.logger.SetLevel(logs.LevelError)
	kv.logger.SetLevel(logs.LevelInformational)
	kv.logger.EnableFuncCallDepth(true) // 输出行号和文件名
	//kv.rf.SetLogger(kv.logger)

	//stopc chan struct{}
	//
	//database map[string]string
	//messagec chan *Message
	//waitMessage map[int]*Message

	kv.db = make(map[string]string)
	kv.result = make(map[int]chan Op)
	kv.stopc = make(chan struct{})
	kv.ack = make(map[int64]int64)
	go kv.getApply()

	return kv
}
