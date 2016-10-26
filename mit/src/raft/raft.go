package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"labrpc"
	"log"
	"math/rand"
	"sync"
	"time"
	//"fmt"
	"bytes"
	"encoding/gob"
	"fmt"
	//"reflect"
	"github.com/astaxie/beego/logs"
	"sort"
)

// 开始对raft进行重构，参考etcd-raft的实现


const None int = -1

// lockedRand is a small wrapper around rand.Rand to provide
// synchronization. Only the methods needed by the code are exposed
// (e.g. Intn).
type lockedRand struct {
	mu   sync.Mutex
	rand *rand.Rand
}

func (r *lockedRand) Intn(n int) int {
	r.mu.Lock()
	v := r.rand.Intn(n)
	r.mu.Unlock()
	return v
}

var globalRand = &lockedRand{
	rand: rand.New(rand.NewSource(time.Now().UnixNano())),
}

type MessageType int32
const (
	MsgVote MessageType = 0
	MsgApp MessageType = 1
	MsgVoteResp MessageType = 2
	MsgAppResp MessageType = 3
	MsgStart MessageType = 4
	MsgState MessageType = 5
)

var MessageType_name = map[int]string{
	0:  "MsgVote",
	1:  "MsgApp",
	2:  "MsgVoteResp",
	3:  "MsgAppResp",
	4:  "MsgStart",
	5:	"MsgState",
}

type Message struct{
	Type MessageType
	From int
	To int
	Term int
	Index int
	isLeader bool
	data interface{}
	svcMeth string
	args interface{}
	reply interface{}
	done chan bool
}
//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

// 状态，leader，follower，candidate
type RaftState int

const (
	StateFollower RaftState = iota
	StateCandidate
	StateLeader
)

func convertStateToString(status RaftState) string {
	switch status {
	case StateFollower:
		return "follower"
	case StateCandidate:
		return "candidate"
	case StateLeader:
		return "leader"
	default:
		return "unknow"
	}
}

type stepFunc func(r *Raft, m *Message)
//type handleFunc func(r *Raft, m Message)

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu                         sync.Mutex
	peers                      []*labrpc.ClientEnd
	persister                  *Persister
	me                         int             // index into peers[]

											   // Your data here.
											   // Look at the paper's Figure 2 for a description of what
											   // state a Raft server must maintain.
											   //
											   // Persistent state on all servers，需要持久化的数据，通过 persister 持久化
											   //
	currentTerm                int             // 当前任期，单调递增，初始化为0
	votedFor                  int              // 当前任期内收到的Candidate号，如果没有为-1
	log                       []Log            // 日志，包含任期和命令，从下标1开始
											   // Volatile state on all servers
											   //
	commitIndex               int              // 已提交的日志，即运用到状态机上
	lastApplied               int              // 已运用到状态机上的最大日志id,初始是0，单调递增
											   //
											   // Volatile state on leaders，选举后都重新初始化
											   //
	nextIndex                 []int            // 记录发送给每个follower的下一个日志index，初始化为leader最大日志+1
	matchIndex                []int            // 记录已经同步给每个follower的日志index，初始化为0，单调递增

											   // 额外的数据
	state                     RaftState        // 当前状态


	electionElapsed           int              //
	heartbeatElapsed          int

	heartbeatTimeout          int
	electionTimeout           int

	randomizedElectionTimeout int

											   // randomizedElectionTimeout 在[electiontimeout, 2 * electiontimeout - 1]之间的一个随机数
											   // 当状态变为follower和candidate都会重置
	heartbeatChan  chan bool
	voteResultChan chan bool        // 选举是否成功
	votedCount     int             // 收到的投票数

	tickc          chan struct{}   // 定时器
	tick           func()
	logger         *logs.BeeLogger // 日志
	lead           int           // 记录当前哪个server是lead
	votes          map[int]bool                         // 记录投票结果
	step           stepFunc
	//handle handleFunc
	commitc        chan int
	//applyCh chan ApplyMsg
	recvc          chan *Message
	propc          chan *Message
	commandc       chan *Message
	stopc          chan struct{}
	replyNextIndex int
}

type Log struct {
	Term    int         // 任期,收到命令时，当前server的任期
	Command interface{} // 命令
	Index   int         // 在数组中的位置
}

// return currentTerm and whether this server
// believes it is the leader.
func (r *Raft) GetState() (int, bool) {
	// 此处没有必要通过 chan 来读取
	// 为什么呢，因为即使你保证了两个状态是一致的，但是在返回给客户端的时候，可能已经变化了
	// 本来就保证不了数据是最新的
	//m := &Message{
	//	Type:MsgState,
	//	done:make(chan bool),
	//}
	//rf.commandc <- m
	//<- m.done
	// 2)Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器
	//go rf.bcastAppend() // 那在哪儿执行应用到状态机的操作呢？在stateMachine中
	//log.Println("broadcastAppendEntriesRPC")
	return r.currentTerm,r.state == StateLeader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.currentTerm) // 当前任期
	e.Encode(rf.log)         // 收到的日志
	e.Encode(rf.votedFor)    // 投票的
	//e.Encode(rf.commitIndex) // 已经确认的一致性日志，之后的日志表示还没有确认是否可以同步，一旦确认的日志都不会改变了
	//e.Encode(rf.lastApplied)
	//e.Encode(rf.nextIndex)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.currentTerm)
	d.Decode(&rf.log)
	d.Decode(&rf.votedFor)
	//d.Decode(&rf.commitIndex)
	//d.Decode(&rf.lastApplied)
	//d.Decode(&rf.nextIndex)
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
					 // Your data here.
	Term         int // candidate的任期
	CandidateId  int // candidate的id
	LastLogIndex int // 候选者最后日志条目的的索引
	LastLogTerm  int // 和lastLogIndex保证至少有一个follower和自己一样持有最新的日志
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
					 // Your data here.
	Term        int  // currentTerm, for candidate to update itself
	VoteGranted bool // true means candidate received vote
}

// 遇到的坑，字段必须大写，不然encode和decode的时候出错
type AppendEntiesArgs struct {
	Term         int   // candidate的任期
	LeaderId     int
	LeaderCommit int   // 已提交的日志，即运用到状态机上
	PrevLogIndex int
	PrevLogTerm  int   // 和prevLogIndex简单地一致性检查
	Entries      []Log // 发送的日志条目
}
type AppendEntiesReply struct {
	Term      int  // currentTerm, for leader to update itself
	Success   bool // true if follower contained entry matching prevLogIndex and prevLogTerm
	NextIndex int  // 为了加快寻找，直接返回下一个需要传递的日志下标
	FollowerCommitIndex int // 调试用
	ConflictTerm int // 不一致的任期
	ConflictIndex int // 不一致任期的最后一个index
}

func (rf *Raft) reqMoreUpToDate(args *RequestVoteArgs) bool {
	// 比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新
	// 如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新
	// 如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。
	lastLog := rf.log[len(rf.log) - 1]
	rlastTerm := args.LastLogTerm
	rlastIndex := args.LastLogIndex
	return (rlastTerm > lastLog.Term) || (rlastTerm == lastLog.Term && rlastIndex >= lastLog.Index)
	//log.Println(lastLog,rlastIndex,rlastIndex)
	//if rlastTerm != lastLog.Term {
	//	return rlastTerm >= lastLog.Term
	//} else {
	//	return rlastIndex >= lastLog.Index
	//}
}

func (rf *Raft) Detail() string {
	detail := fmt.Sprintf("server:%d,currentTerm:%d,role:%s\n", rf.me, rf.currentTerm, convertStateToString(rf.state))
	detail += fmt.Sprintf("commitIndex:%d,lastApplied:%d,len(log):%d\n", rf.commitIndex, rf.lastApplied,len(rf.log))
	detail += fmt.Sprintf("log is:%v\n", rf.log[rf.lastApplied:])
	detail += fmt.Sprintf("nextIndex is:%v\n", rf.nextIndex)
	detail += fmt.Sprintf("matchIndex is:%v\n", rf.matchIndex)
	return detail
}

// Receiver implementation
// + 通用规则：如果Rpc请求或回复包括纪元T > currentTerm: 设置currentTerm = T,转换成 follower
// 如果term < currentTerm 则返回false
// 如果本地的voteFor为空或者为candidateId,并且候选者的日志至少与接受者的日志一样新,则投给其选票

func (rf *Raft)RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	m := &Message{
		Type:MsgVote,
		From:args.CandidateId,
		To:rf.me,
		Term:args.Term,
		svcMeth:"Raft.RequestVote",
		args:args,
		reply:reply,
		done:make(chan bool),
	}
	rf.recvc <- m
	<- m.done
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// 外部的调用
func (r *Raft) AppendEnties(args AppendEntiesArgs, reply *AppendEntiesReply) {
	m := &Message{
		Type:MsgApp,
		From:args.LeaderId,
		To:r.me,
		Term:args.Term,
		svcMeth:"Raft.AppendEnties",
		args:args,
		reply:reply,
		done:make(chan bool),
	}
	//r.logger.Notice("%d [term:%d] refuse %d [term:%d] [message:%d]",m.To,r.currentTerm,m.From,m.Term,*m)
	r.recvc <- m
	<- m.done
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
// 如果返回false，表示网络错误，没有发送成功
//func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
//	return true
//}

func (r *Raft)commitTo(tocommit int){
	if tocommit > r.lastIndex() {
		r.logger.Error("new commitIndex:%d is bigger than lastIndex:%d",tocommit,r.lastIndex())
		panic(" tocommit > r.lastIndex() ")
	}

	if r.commitIndex < tocommit {
		//r.logger.Notice("%d update commit:%d to %d",r.me, r.commitIndex,tocommit)
		r.commitIndex = tocommit
		r.commitc <- tocommit
	}else {
		r.logger.Error("new commitIndex:%d is smaller than older commitIndex:%d len(rf.log):%d",tocommit,r.commitIndex, len(r.log))
	}
}

func (rf *Raft) updateCommitIndex() {
	// 如果存在一个N满足N>commitIndex,多数的matchIndex[i] >= N,并且 log[N].term == currentTerm:设置commitIndex = N
	// 找出 matchIndex（已经同步给各个follower的值）
	//newCommitIndex := rf.commitIndex
	//count := 0

	mis := make(sort.IntSlice,0,len(rf.peers))
	for _,v := range rf.matchIndex {
		mis = append(mis,v)
	}
	sort.Sort(sort.Reverse(mis))
	mci := mis[rf.quorum()-1] // 应该是从大到小排序
	if mci > rf.commitIndex && rf.log[mci].Term == rf.currentTerm {
		// 此处由于
		//rf.logger.Info("%v,mci:%d,commitIndex:%d",mis,mci,rf.commitIndex)
		// 此时只能说 newCommitIndex 具备了能提交的可能性，但是还要保证 log[newCommitIndex].term == currentTerm 才能提交
		// 原因参见5.4.2
		//log.Println("leader:",rf.me,"update commitIndex to",newCommitIndex)
		//rf.logger.Notice("leader:%d update commitIndex to:%d, matchIndex:%v",rf.me,mci,rf.matchIndex)
		rf.commitTo(mci)
	}
}


//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
// 通过start来执行命令，如果当前server不是leader，返回false
// 函数需要立即返回，但是不保证这个命令已经持久化到了日志中，然后开启agreement过程
// 第一个返回值表示如果命令提交，下标将会是多少
// 第二个返回值是当前的任期
// 第三个返回值表明当前server是否是leader
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {

	index := -1
	term := -1
	isLeader := true

	if rf.state != StateLeader {
		isLeader = false
		return index, term, isLeader
	}
	// 只有是leader了，才串行的进行操作
	m := &Message{
		Type:MsgStart,
		data:command,
		done:make(chan bool),
	}
	rf.commandc <- m
	<- m.done
	// 2)Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器
	//go rf.bcastAppend() // 那在哪儿执行应用到状态机的操作呢？在stateMachine中
	//log.Println("broadcastAppendEntriesRPC")
	return m.Index, m.Term, m.isLeader
}
//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
	close(rf.stopc)
}
func (rf *Raft)resetRandomizedElectionTimeout() {
	rf.randomizedElectionTimeout = rf.electionTimeout + globalRand.Intn(rf.electionTimeout)
}


// 此处好的方法是stateMachine和其他通过channel通道
func (rf *Raft) stateMachine(applyCh chan ApplyMsg) {
	// 在此运用已经agree的日志到状态机

	for {
		select {
		case commitIndex := <- rf.commitc:
			if commitIndex > rf.lastApplied {
				if rf.lastIndex() < commitIndex {
					// 这种情况出现时不正常的，因为commitIndex应该是leader发过来的，最大不会超过rf.log
					// 为什么会出现这个，大家想下，follower中如果commitIndex是server发过来的，可能出现commitIndex大，但是log还没有发送过来的情况
					//log.Fatal("machine!! ", rf.Detail())
					rf.logger.Error("%s",rf.Detail())
					panic("stateMachine error")
				}
				lastApplied := rf.lastApplied
				for i := lastApplied + 1; i <= commitIndex; i++ {
					applymsg := ApplyMsg{
						Index:   rf.log[i].Index,
						Command: rf.log[i].Command,
					}
					//rf.logger.Notice("%d apply index:%d to app",rf.me,i)
					applyCh <- applymsg // 此处可能阻塞
				}
				rf.lastApplied = commitIndex
				// 应用完状态机后，更新rf.lastApplied
			}
		case <- rf.stopc:
			return
			//rf.logger.Info("%d stop",rf.me)
		}
	}
}
func (r *Raft) pastElectionTimeout() bool {
	return r.electionElapsed >= r.randomizedElectionTimeout
}
func (raft *Raft)tickElection() {
	raft.electionElapsed++
	if raft.pastElectionTimeout() {
		raft.electionElapsed = 0
		raft.campaign()
	}
}
func (r *Raft) tickHeartbeat() {
	r.heartbeatElapsed++
	if r.heartbeatElapsed >= r.heartbeatTimeout {
		r.heartbeatElapsed = 0
		r.bcastAppend()
	}
}

// 请求投票
func (rf *Raft)bcastRequestVoteRPC() {
	// 发送rpc之前
	rf.persist()
	// 发送消息给给各个peer
	for i := range rf.peers {
		if i != rf.me {
			args := RequestVoteArgs{
				Term:         rf.currentTerm,
				CandidateId:  rf.me,
				LastLogIndex: rf.lastIndex(),
				LastLogTerm:  rf.lastTerm(),
			}
			m := &Message{
				Type:MsgVote,
				From:rf.me,
				To:i,
				Term:rf.currentTerm,
				svcMeth:"Raft.RequestVote",
				args:args,
				reply:&RequestVoteReply{},
				done:make(chan bool),
			}
			go func(m *Message) {
				rf.propc <- m
				ok := <- m.done
				if ok {
					// 处理结果
					respm := &Message{
						Type:MsgVoteResp,
						From:m.To,
						To:m.From,
						Term:m.reply.(*RequestVoteReply).Term,
						args:m.args,
						reply:m.reply,
						done:make(chan bool),
					}
					rf.recvc <- respm
					//<- m.done
				}else {
					// TODO:对于失败的请求是否重试？
				}
			}(m)
		}
	}
}
//
func (rf *Raft)bcastAppend(){
	rf.persist()
	// 发送消息给给各个peer
	for i := range rf.peers {
		if i != rf.me {
			// ！！！由于此处使用goroutine,有可能后面的请求先到server，即大的rf.commitIndex先到，反而前面的请求后到，需要在处理端处理
			prevLogIndex := rf.nextIndex[i] - 1
			if prevLogIndex < 0 || prevLogIndex  > rf.lastIndex() {
				rf.logger.Error("Detail:%s", rf.Detail())
				panic("prevLogIndex < 0 || prevLogIndex  > rf.lastIndex() ")
			}
			args := AppendEntiesArgs{
				Term:         rf.currentTerm,
				LeaderId:     rf.me,
				LeaderCommit: rf.commitIndex,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  rf.log[prevLogIndex].Term,
				Entries:      rf.log[prevLogIndex + 1:], // 如果没有新日志，自然而然是空
			}
			//rf.logger.Alert("leader:%d args.entries:%v log:%v",rf.me,args.Entries,rf.log)
			m := &Message{
				Type:MsgApp,
				From:rf.me,
				To:i,
				Term:rf.currentTerm,
				svcMeth:"Raft.AppendEnties",
				args:args,
				reply:&AppendEntiesReply{},
				done:make(chan bool),
			}
			go func(m *Message) {
				rf.propc <- m
				ok := <- m.done
				if ok {
					// 处理结果
					respm := &Message{
						Type:MsgAppResp,
						From:m.To,
						To:m.From,
						Term:m.reply.(*AppendEntiesReply).Term,
						args:args,
						reply:m.reply,
						done:make(chan bool),
					}
					rf.recvc <- respm
					//<- m.done // 不关注结果
				}else {
					//TODO:失败处理
				}
			}(m)
		}
	}
}


func (r *Raft)lastIndex() int {
	return r.log[len(r.log)-1].Index
}
func (r *Raft)lastTerm() int{
	return r.log[r.lastIndex()].Term
}

func (r *Raft)matchTerm(index, term int)bool {
	//if index > r.lastIndex() {
	//	r.logger.Error("index:%d lastIndex:%d commitIndex:%d",index,r.lastIndex(),r.commitIndex)
	//}
	//if index >= r.lastIndex() {
	//	if index > len(r.log) -1 {
	//		r.logger.Error("index:%d lastIndex:%d r.log:%v",index,r.lastIndex(),r.log)
	//	}
	//}
	return index <= r.lastIndex() && r.log[index].Term == term
}

func (r *Raft)term(index int)int {
	return r.log[index].Term
}

func (r *Raft)findConflict(ents []Log) int{
	for _,ne := range ents {
		if !r.matchTerm(ne.Index,ne.Term){
			if ne.Index <= r.lastIndex() {
				r.logger.Info("%d found conflict at index %d [existing term: %d, conflicting term: %d]",
					r.me, ne.Index, r.term(ne.Index), ne.Term)
				//r.logger.Info("%v",ents)
			}
			return ne.Index
		}
	}
	return 0
}

func (r*Raft)appendLog(ents ...Log)int{
	if len(ents) == 0 {
		return r.lastIndex()
	}
	if after := ents[0].Index - 1; after < r.commitIndex {
		r.logger.Error("after(%d) is out of range [committed(%d)]", after, r.commitIndex)
		panic("appendLog")
	}
	r.log = r.log[:ents[0].Index]
	r.log = append(r.log,ents...)
	return r.lastIndex()
}

func (r *Raft)handleAppendEntries(m *Message) {
	//r.logger.Notice("%d receive heart beat from %d",m.To,m.From)
	defer func() {
		//检查，强校验
		reply := m.reply.(*AppendEntiesReply)
		//args := m.args.(AppendEntiesArgs)
		if reply.NextIndex > r.lastIndex() + 1 {
			//r.logger.Error("args:%v",args)
			//r.logger.Alert("follow:%d args.entries:%v log:%v",r.me,args.Entries,r.log)
			r.logger.Error("reply.NextIndex:%d > rf.lastIndex():%d + 1",reply.NextIndex,r.lastIndex())
			panic("reply.NextIndex > rf.lastIndex() + 1")
		}
	}()

	args := m.args.(AppendEntiesArgs)
	reply := m.reply.(*AppendEntiesReply)
	//if m.Term < rf.currentTerm {
	//	reply.Term = rf.currentTerm
	//	reply.Success = false
	//	return
	//}
	r.electionElapsed = 0
	r.lead = m.From
	//if args.PrevLogIndex < rf.commitIndex || args.LeaderCommit < rf.commitIndex {
	if args.PrevLogIndex < r.commitIndex {
		// 此处不加上args.LeaderCommit < rf.commitIndex，因为新leader的请求就是会出现小于follower的情况
		// 此处是过期的请求，可能会出现过期的请求，
		// 但是一旦出现 args.LeaderCommit < rf.commitIndex，代表的不是过期，而是 leader的commiIndex更新太慢了，要快速更新
		//if args.LeaderCommit < rf.commitIndex {
		//	rf.logger.Error("leader:%d has older commmit:%d than follower:%d commit:%d",args.LeaderId,args.LeaderCommit,rf.me,rf.commitIndex)
		//	panic("args.LeaderCommit < rf.commitIndex")
		//}
		// 如果此处 PrevLogIndex = 440, rf.commitIndex = 441
		// 则 reply.NextIndex = 442
		// len(args.Entries) == 0, args.LeaderCommit = 439
		// leader lastIndex:440，可能吗？如果出现这种错误，就是选举的时候出现了错误

		// 乱序的请求，返回正确的 commitIndex
		//rf.logger.Info("follower:%d receives unordered appendrpc",rf.me)
		reply.Term = r.currentTerm
		reply.Success = true
		reply.NextIndex = r.commitIndex + 1
		reply.FollowerCommitIndex = r.commitIndex
		//reply.NextIndex = max(rf.replyNextIndex,rf.commitIndex+1)
		return
	}
	if r.matchTerm(args.PrevLogIndex,args.PrevLogTerm){
		// 因为此处可能后传来的日志 PrevLogIndex + len(args.Entries) 少了，这个怎么办呢？
		nexti := args.PrevLogIndex + len(args.Entries)  + 1
		//lastnewi := args.PrevLogIndex + len(args.Entries)
		ci := r.findConflict(args.Entries)

		switch  {
		case ci == 0:
		case ci <= r.commitIndex:
			r.logger.Error("entry %d conflict with committed entry [committed(%d)]", ci, r.commitIndex)
			panic("ci <= r.commitIndex")
		default:
			offset := args.PrevLogIndex + 1
			r.appendLog(args.Entries[ci-offset:]...)
		}

		// 此处为什么不直接返回 len(rf.log)，因为可能 args.Entries == 0，但是本地的log日志长度长
		//if len(args.Entries) > 0 {
		//	r.log = r.log[:args.PrevLogIndex+1]
		//	r.log = append(r.log,args.Entries...)
		r.checkLog(1)
		//}
		if args.LeaderCommit > r.commitIndex {
			if args.LeaderCommit > r.lastIndex() {
				r.logger.Error("%d [LeaderCommit:%d PrevLogIndex:%d len(entries):%d]",args.LeaderId,args.PrevLogIndex,len(args.Entries))
				panic("args.LeaderCommit > rf.lastIndex()")
			}
			//rf.commitTo(min(args.LeaderCommit, rf.lastIndex()))
			r.commitTo(args.LeaderCommit)
		}else {
			// 此处还是可能是过期的请求
			//if args.LeaderCommit < rf.commitIndex {
			//	//rf.logger.Error("leader:%d has older commmit:%d than follower:%d commit:%d",args.LeaderId,args.LeaderCommit,rf.me,rf.commitIndex)
			//	rf.logger.Error("leader:%d PrevLogIndex:%d has older commmit:%d than follower:%d commit:%d",args.LeaderId,args.PrevLogIndex, args.LeaderCommit,rf.me,rf.commitIndex)
			//	panic("args.LeaderCommit < rf.commitIndex")
			//}
		}
		reply.Term = r.currentTerm
		reply.Success = true
		reply.NextIndex = nexti
		// 此处就是会不相等，因为老的请求会让的log变短
		//if nexti != len(r.log) {
		//	r.logger.Error("PrevLogIndex:%d len(args.Entries):%d len(r.log):%d",args.PrevLogIndex,len(args.Entries),len(r.log))
		//	panic("append error")
		//}
	}else {
		// 优化,怎么快速回滚？
		// 如果follower拒绝了，那回复中包含：
		// 不一致entry的任期（term）
		// 不一致任期的第一个entry的下标
		// 如果leader知道不一致的任期：
		//​ 将nextIndex[i]设置为不一致任期的最后一个entry
		// 否则：
		//​ 将nextIndex[i]设置为follower返回的index
		// 如果此时都不存在 PrevLogIndex，则返回别的
		if args.PrevLogIndex <= r.lastIndex() {
			reply.ConflictTerm = r.log[args.PrevLogIndex].Term // Term > 0
			ConflictIndex := 0
			for ConflictIndex:=args.PrevLogIndex; ConflictIndex>0;ConflictIndex-- {
				if r.log[ConflictIndex-1].Term != reply.ConflictTerm {
					break
				}
			}
			reply.ConflictIndex = ConflictIndex
		}else {
			reply.ConflictTerm = -1
			reply.ConflictIndex = -1
		}

		//rf.logger.Info("follow:%d matchTerm(%d,%d) fail, len(log):%d commitIndex:%d",rf.me,args.PrevLogIndex,args.PrevLogTerm,len(rf.log),rf.commitIndex)
		reply.Term = r.currentTerm
		reply.Success = false
		// if PrevLogIndex = 1 and commitIndex = 0,
		// then NextIndex = 0
		reply.NextIndex = min(r.commitIndex + 1, args.PrevLogIndex - 1) //
		if reply.NextIndex < 1 {
			reply.NextIndex = 1
		}
	}
	if reply.NextIndex < 1 {
		r.logger.Error("args.PrevLogIndex:%d,args.PrevLogTerm:%d,rf.log:%v",args.PrevLogIndex,args.PrevLogTerm, r.log)
		panic("reply.NextIndex < 1")
	}
	r.replyNextIndex = reply.NextIndex
	reply.FollowerCommitIndex = r.commitIndex
}

// follower收到投票请求和append
func stepFollower(rf *Raft, m *Message){
	//defer close(m.done)
	switch m.Type {
	case MsgVote:
		//func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
		args := m.args.(RequestVoteArgs)
		reply := m.reply.(*RequestVoteReply)
		if (rf.votedFor == None || rf.votedFor == args.CandidateId) && rf.reqMoreUpToDate(&args){
			rf.electionElapsed = 0
			rf.logger.Info("%x [logterm: %d, index: %d, vote: %x] voted for %x [logterm: %d, index: %d] at term %d",
				rf.me, rf.lastTerm(), rf.lastIndex(), rf.votedFor, args.CandidateId, args.LastLogTerm, args.LastLogIndex, rf.currentTerm)
			rf.votedFor = args.CandidateId
			reply.Term = rf.currentTerm
			reply.VoteGranted = true

		}else {
			rf.logger.Info("%x [logterm: %d, index: %d, vote: %x] refuse for %x [logterm: %d, index: %d] at term %d",
				rf.me, rf.lastTerm(), rf.lastIndex(), rf.votedFor, args.CandidateId, args.LastLogTerm, args.LastLogIndex, rf.currentTerm)

			reply.Term = rf.currentTerm
			reply.VoteGranted = false
		}
	case MsgApp:
		rf.handleAppendEntries(m)
	default:
		// 不是自己关注的命令，怎么办
		rf.logger.Debug("follower:%x receives unsupported message:%s",rf.me,MessageType_name[int(m.Type)])
	}
}

// stepCandidate 能收到
func stepCandidate(rf *Raft, m *Message){
	//defer close(m.done)
	switch m.Type {
	case MsgApp:
		rf.becomeFollower(rf.currentTerm,m.From)
		rf.handleAppendEntries(m)
	case MsgVote:
		args := m.args.(RequestVoteArgs)
		reply := m.reply.(*RequestVoteReply)
		rf.logger.Info("%x [logterm: %d, index: %d, vote: %x] rejected vote from %x [logterm: %d, index: %d] at term %d",
			rf.me, rf.lastTerm(), rf.lastIndex(), rf.votedFor, m.From, args.LastLogTerm, args.LastLogIndex, rf.currentTerm)
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
	case MsgVoteResp:
		// 处理投票结果
		//args := m.args.(RequestVoteArgs)
		reply := m.reply.(*RequestVoteReply)
		//if reply.Term < rf.currentTerm {
		//	// 过时的投票，ignore
		//}else {
			gr := rf.poll(m.From,reply.VoteGranted)
			rf.logger.Info("%x [quorum:%d] has received %d votes and %d vote rejections", rf.me, rf.quorum(), gr, len(rf.votes)-gr)
			switch  rf.quorum(){
			case gr: // 半数同意
				rf.becomeLeader()
				rf.bcastAppend()
			case len(rf.votes)-gr:// 半数拒绝
				rf.becomeFollower(rf.currentTerm,None)
			}
		//}
	default:
		// 不是自己关注的命令，怎么办
		rf.logger.Debug("candidate:%x receives unsupported message:%d",rf.me,m.Type)
	//case MsgApp:
	//	// 可能收到吗？
	}
}

func stepLeader(r *Raft, m *Message){
	//defer close(m.done)
	switch m.Type {
	case MsgAppResp:
		//r.logger.Notice("%d handle MsgAppResp from %d log:%v",r.me,m.From,r.log)
		args := m.args.(AppendEntiesArgs)
		reply := m.reply.(*AppendEntiesReply)
		//if (r.nextIndex[m.From] >= reply.NextIndex) {
		//	return
		//}
			//if args.PrevLogIndex < rf.commitIndex || args.LeaderCommit < rf.commitIndex {
			// 此处是过期的请求，可能会出现，过期的请求
		if reply.NextIndex > r.lastIndex()+1 {
			r.logger.Error("MsgAppResp reject:%v [PrevLogIndex:%d PrevLogTerm:%d commitIndex:%d Entries's len:%d]",!reply.Success, args.PrevLogIndex,args.PrevLogTerm,r.commitIndex,len(args.Entries))
			r.logger.Error("%d [term:%d nextIndex:%d commitIndex:%d] is bigger than leader:%d [term:%d lastIndex:%d]",m.From,m.Term ,reply.NextIndex,reply.FollowerCommitIndex, r.me,r.currentTerm,r.lastIndex())
			r.checkLog("reply.NextIndex > r.lastIndex()+1")
			panic("reply.NextIndex > r.lastIndex()+1")
		}
		if reply.Success { // 匹配成功
			//r.logger.Notice("%d accept %d [reply.NextIndex:%d r.nextIndex:%d]",m.From,m.To,reply.NextIndex,r.nextIndex[m.From])
			// 如果 reply.NextIndex < r.nextIndex[m.From]，意味着已经跟新过了 matchIndex 了，无需再次更新
			if reply.NextIndex >= r.nextIndex[m.From] {
				r.nextIndex[m.From] = reply.NextIndex
				r.matchIndex[m.From] = reply.NextIndex - 1
				r.updateCommitIndex()
			}
			// 如果此时matchIndex都没有更新，有问题，因此matchIndex的更新时机是此处
		} else {
			//r.logger.Notice("%d refuse %d PrevLogIndex:%d PrevLogTerm:%d",m.From,m.To,args.PrevLogIndex,args.PrevLogTerm)
			// 优化：快速next
			// 优化,怎么快速回滚？
			// 如果follower拒绝了，那回复中包含：
			// 不一致entry的任期（term）
			// 不一致任期的第一个entry的下标
			// 如果leader知道不一致的任期：
			//​ 将nextIndex[i]设置为不一致任期的最后一个entry
			// 否则：
			//​ 将nextIndex[i]设置为follower返回的index
			nexti := 0
			for i:=args.PrevLogIndex-1;i>0;i-- {
				if r.log[i].Term == reply.ConflictTerm {
					// 找到了第一个不一致的任期
					nexti = i
					break
				}
			}
			if nexti == 0 && reply.ConflictIndex > 0{
				r.nextIndex[m.From] = reply.ConflictIndex
			} else {
				r.nextIndex[m.From] = reply.NextIndex
			}
		}
	case MsgVoteResp:
		r.logger.Info("leader:%d [term:%d] receives overdue MsgVoteResp from %d [term:%d]",r.me,r.currentTerm,m.From,m.Term)
	default:
		// 不是自己关注的命令，怎么办
		r.logger.Debug("leader:%x receives unsupported message:%s",r.me,MessageType_name[int(m.Type)])
	}
}

// 处理收到的请求和响应
func (r *Raft)Step(m *Message) error{
	defer func() {
		// 在回复之前持久化和处理完响应后
		r.persist()
		close(m.done)
	}()

	r.logger.Debug("%d received msg:%s from %d at term:%d",m.To,MessageType_name[int(m.Type)],m.From,m.Term)
	switch {
	case m.Term > r.currentTerm:
		r.logger.Info("%d:%s [term:%d] smaller than %d [term:%d] become follower",r.me, convertStateToString(r.state) ,r.currentTerm,m.From,m.Term)
		r.becomeFollower(m.Term,None)
	case m.Term < r.currentTerm:
		// 过期的请求和响应，just response with false
		switch m.Type {
		case MsgApp:
			reply := m.reply.(*AppendEntiesReply)
			//args := m.args.(AppendEntiesArgs)
			//r.logger.Notice("%d [term:%d] refuse %d [term:%d] [message:%d]",m.To,r.currentTerm,m.From,m.Term,*m)
			reply.Term = r.currentTerm
			reply.NextIndex = r.commitIndex+1
			reply.Success = false
		case MsgVote:
			reply := m.reply.(*RequestVoteReply)
			reply.Term = r.currentTerm
			reply.VoteGranted = false
		}
		return nil
	}
	r.step(r,m)
	return nil
}

func (r *Raft) quorum() int { return len(r.peers)/2 + 1 }


// 开始选举
func (r *Raft) campaign() {
	r.becomeCandidate()
	if r.quorum() == r.poll(r.me,true){
		r.becomeLeader()
		return
	}
	// 给每一个线程都发送消息
	r.bcastRequestVoteRPC()
}

func (r *Raft)serveChannels() {
	for {
		select {
		case m := <-r.propc: // 读取发送命令
			go func(m *Message) {
				r.logger.Debug("%d send %s to %d",m.From,m.svcMeth,m.To)
				ok := r.peers[m.To].Call(m.svcMeth,m.args,m.reply)
				m.done <- ok
			}(m)
		case <- r.stopc:
			return
			//r.logger.Info("%d stop",r.me)
		}
	}
}
//
//func (r *Raft)applyCommitLog() {
//	if r.commitIndex > r.lastApplied {
//		for i:=r.lastApplied;i<=r.commitIndex;i++ {
//			r.applyCh <- ApplyMsg{Index:r.log[i].Index,Command:r.log[i].Command}
//		}
//	}
//}
func (r *Raft)handleStart(m *Message) {
	//r.logger.Notice("%d handle start command",r.me)
	if r.state != StateLeader {
		m.Index = -1
		m.Term = -1
		m.isLeader = false
		m.done <- true
	}else {
		command := m.data
		term := r.currentTerm
		index := len(r.log)
		//panic(fmt.Sprint(command))
		r.log = append(r.log,Log{Term:term,Index:index,Command:command})
		m.Term = term
		m.Index = index
		m.isLeader = true
		r.nextIndex[r.me] = index+1
		r.matchIndex[r.me] = index

		// 新增日志，进行持久化,回复给 start rpc的时候
		// 发送 send 之前
		//r.persist()
		r.bcastAppend()
		m.done <- true
	}
}
func (r *Raft)handleState(m *Message) {
	m.isLeader = r.state == StateLeader
	m.Term = r.currentTerm
	m.done <- true
}
func (r *Raft)handleCommand(m *Message){
	switch m.Type {
	case MsgStart:
		r.handleStart(m)
	case MsgState:
		r.handleState(m)
	default:
		panic("un supported command")
	}
}

func (r *Raft) Tick() {
	select {
	case r.tickc <- struct{}{}:
	//case <-r.stopc:
	default:
		r.logger.Error("A tick missed to fire. Node blocks too long!")
		panic("A tick missed to fire. Node blocks too long!")
	}
}

func (r *Raft)run() {

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 100ms
		defer ticker.Stop() // 定时发生器
		for {
			select {
			case <- ticker.C:
				r.Tick()
			case <- r.stopc:
				return
			}
		}
	}()

	for {
		// 写操作通过channel进行了强制排队
		select {
		case <- r.tickc:
			r.tick()
		case m := <- r.recvc: // 收到消息和回复
			r.Step(m)
		case m := <- r.commandc: // 进行 start 命令
			r.handleCommand(m) // 此处完全可以通过 goroutine 执行,出现错误
		case <- r.stopc:
			return
		}
	}
}


// 给自己设置投票结果，并且得到支持的票数
func (r *Raft) poll(id int, v bool) (granted int) {
	if v {
		r.logger.Info("%x received vote from %x at term %d", r.me, id, r.currentTerm)
	} else {
		r.logger.Info("%x received vote rejection from %x at term %d", r.me, id, r.currentTerm)
	}
	if _, ok := r.votes[id]; !ok {
		r.votes[id] = v
	}
	for _, vv := range r.votes {
		if vv {
			granted++
		}
	}
	return granted
}

func (raft *Raft)reset(term int) {
	if raft.currentTerm != term{
		raft.currentTerm = term
		raft.votedFor = None
	}
	raft.lead = None
	raft.heartbeatElapsed = 0
	raft.electionElapsed = 0
	raft.resetRandomizedElectionTimeout()
	raft.votes = make(map[int]bool)
	//raft.commitIndex = 0
	raft.replyNextIndex = 1
	if raft.log == nil {
		raft.log = []Log{{Term: 1, Index: 0}}
	}
	raft.nextIndex = make([]int,len(raft.peers))
	raft.matchIndex = make([]int,len(raft.peers))
}

func (raft *Raft)becomeFollower(term int, lead int) {
	raft.reset(term)
	raft.tick = raft.tickElection
	raft.state = StateFollower
	raft.lead = lead
	raft.step = stepFollower
	raft.logger.Info("%x became follower at term %d", raft.me, raft.currentTerm)
}

func (r *Raft)becomeCandidate() {
	if r.state == StateLeader {
		panic("invalid transition [leader -> candidate]")
	}
	r.reset(r.currentTerm+1)
	r.tick = r.tickElection
	r.votedFor = r.me
	r.state = StateCandidate
	r.step = stepCandidate
	r.logger.Info("%x became candidate at term %d", r.me, r.currentTerm)
}
func (r *Raft) becomeLeader() {
	if r.state == StateFollower {
		panic("invalid transition [follower -> leader]")
	}
	r.reset(r.currentTerm)
	r.lead = r.me
	r.state = StateLeader
	r.tick = r.tickHeartbeat
	r.step = stepLeader
	for i := 0; i < len(r.peers); i++ {
		r.nextIndex[i] = len(r.log)
		// 此处不应该更新 r.matchIndex[i]，只有当真正匹配的时候才更新
		//r.matchIndex[i] = r.nextIndex[i] - 1
	}
	r.logger.Info("%x became leader at term %d commitIndex:%d", r.me, r.currentTerm, r.commitIndex)
}

func (r *Raft)SetLogger(logger *logs.BeeLogger) {
	r.logger = logger
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
// 所有服务器的peers都有相同的顺序
// persister是对持久化的抽象
// applyCh是传递消息的通道，当每次执行命令后，通过applyCh通道来传递消息
// Make()需要快速的返回，如果有长耗时任务，需要启动goroutine来完成
func Make(peers []*labrpc.ClientEnd, me int,
persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	//ticker := time.NewTicker(100 * time.Millisecond) // 100ms
	//defer ticker.Stop() // 定时发生器
	// 新的初始化函数
	rf.electionTimeout = 10
	rf.heartbeatTimeout = 5
	rf.logger = logs.NewLogger()
	rf.logger.SetLogger(logs.AdapterConsole)
	//rf.logger.SetLevel(logs.LevelError)
	//rf.logger.SetLevel(logs.LevelInformational)
	rf.logger.SetLevel(logs.LevelNotice)
	rf.logger.EnableFuncCallDepth(true) // 输出行号和文件名

	//rf.applyCh = applyCh
	rf.recvc = make(chan *Message)
	rf.commitc = make(chan int)
	rf.commandc = make(chan *Message)
	rf.propc = make(chan *Message)
	rf.stopc = make(chan struct{})
	rf.tickc = make(chan struct{}, 10)

	rf.becomeFollower(1,None)
	rf.readPersist(persister.ReadRaftState())
	//rf.logger.Info("log:%v",rf.log)
	go rf.run()
	go rf.serveChannels()
	go rf.stateMachine(applyCh)
	return rf
}

func (rf *Raft) checkLog(v interface{}) {
	for i := 0; i < len(rf.log); i++ {
		if i != rf.log[i].Index {
			//log.Println("check log type:", reflect.TypeOf(v).String(), "value:", v)
			log.Fatal("error log index", rf.Detail())
		}
	}
}
