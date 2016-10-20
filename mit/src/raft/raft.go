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
	"sync"
	"labrpc"
	"log"
	"math/rand"
	"time"
	//"fmt"
	"fmt"
	"bytes"
	"encoding/gob"
	"reflect"
)

// 开始对raft进行重构，参考etcd-raft的实现

// import "bytes"
// import "encoding/gob"

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
type RaftStatus int

const (
	STATUS_FOLLOWER RaftStatus = iota
	STATUS_CANDIDATE
	STATUS_LEADER
)

func convertStatusToString(status RaftStatus) string {
	switch status {
	case STATUS_FOLLOWER:
		return "follower"
	case STATUS_CANDIDATE:
		return "candidate"
	case STATUS_LEADER:
		return "leader"
	default:
		return "unknow"
	}
}


//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu                        sync.Mutex
	peers                     []*labrpc.ClientEnd
	persister                 *Persister
	me                        int           // index into peers[]

											// Your data here.
											// Look at the paper's Figure 2 for a description of what
											// state a Raft server must maintain.
											//
											// Persistent state on all servers，需要持久化的数据，通过 persister 持久化
											//
	currentTerm               int           // 当前任期，单调递增，初始化为0
	votedFor                  int           // 当前任期内收到的Candidate号，如果没有为-1
	log                       []Log         // 日志，包含任期和命令，从下标1开始
											// Volatile state on all servers
											//
	commitIndex               int           // 已提交的日志，即运用到状态机上
	lastApplied               int           // 已运用到状态机上的最大日志id,初始是0，单调递增
											//
											// Volatile state on leaders，选举后都重新初始化
											//
	nextIndex                 []int         // 记录发送给每个follower的下一个日志index，初始化为leader最大日志+1
	matchIndex                []int         // 记录已经同步给每个follower的日志index，初始化为0，单调递增

											// 额外的数据
	status                    RaftStatus    // 当前状态
	electionTimeout           time.Duration // 选举超时时间
	heartbeatTimeout          time.Duration // 心跳超时时间
	randomizedElectionTimeout time.Duration // 随机选举超时时间
											// randomizedElectionTimeout 在[electiontimeout, 2 * electiontimeout - 1]之间的一个随机数
											// 当状态变为follower和candidate都会重置
	heartbeatChan             chan bool
	voteResultChan            chan bool     // 选举是否成功
	votedCount                int           // 收到的投票数
}

type Log struct {
	Term    int         // 任期,收到命令时，当前server的任期
	Command interface{} // 命令
	Index   int         // 在数组中的位置
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isLeader bool
	// Your code here.
	term = rf.currentTerm
	isLeader = rf.status == STATUS_LEADER
	return term, isLeader
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
	e.Encode(rf.log) // 收到的日志
	e.Encode(rf.votedFor) // 投票的
	e.Encode(rf.commitIndex) // 已经确认的一致性日志，之后的日志表示还没有确认是否可以同步，一旦确认的日志都不会改变了
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
	d.Decode(&rf.commitIndex)
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
type    AppendEntiesArgs struct {
	Term         int   // candidate的任期
	LeaderId     int
	LeaderCommit int   // 已提交的日志，即运用到状态机上
	PrevLogIndex int
	PrevLogTerm  int   // 和prevLogIndex简单地一致性检查
	Entries      []Log // 发送的日志条目
}
type AppendEntiesReply struct {
	Term    int  // currentTerm, for leader to update itself
	Success bool // true if follower contained entry matching prevLogIndex and prevLogTerm
	NextIndex int // 为了加快寻找，直接返回下一个需要传递的日志下标
}

func (rf *Raft)reqMoreUpToDate(args *RequestVoteArgs) bool {
	// 比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新
	// 如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新
	// 如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。
	lastLog := rf.log[len(rf.log) - 1]
	rlastTerm := args.LastLogTerm
	rlastIndex := args.LastLogIndex
	//log.Println(lastLog,rlastIndex,rlastIndex)
	if rlastTerm != lastLog.Term {
		return rlastTerm >= lastLog.Term
	}else {
		return rlastIndex >= lastLog.Index
	}
}

func (rf *Raft)Detail() string {
	detail := fmt.Sprintf("server:%d,currentTerm:%d,role:%s\n", rf.me, rf.currentTerm, convertStatusToString(rf.status))
	detail += fmt.Sprintf("commitIndex:%d,lastApplied:%d\n", rf.commitIndex, rf.lastApplied)
	detail += fmt.Sprintf("log is:%v\n", rf.log)
	detail += fmt.Sprintf("nextIndex is:%v\n", rf.nextIndex)
	detail += fmt.Sprintf("matchIndex is:%v\n", rf.matchIndex)
	return detail
}

func (rf *Raft)rpcRuleForAllServer(term int) {
	if term > rf.currentTerm {
		rf.currentTerm = term
		rf.status = STATUS_FOLLOWER
		rf.votedFor = -1
	}
}
// Receiver implementation
// + 通用规则：如果Rpc请求或回复包括纪元T > currentTerm: 设置currentTerm = T,转换成 follower
// 如果term < currentTerm 则返回false
// 如果本地的voteFor为空或者为candidateId,并且候选者的日志至少与接受者的日志一样新,则投给其选票

func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	rf.rpcRuleForAllServer(args.Term)

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) && rf.reqMoreUpToDate(&args){
		rf.votedFor = args.CandidateId
		reply.Term = rf.currentTerm
		reply.VoteGranted = true
		return
	}else {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}
	//log.Println("follower:",rf.me,"deal RequestVote done, voteGranted:", reply.VoteGranted)
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
//
func (rf *Raft) AppendEnties(args AppendEntiesArgs, reply *AppendEntiesReply) {
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}
	// 本身自己是leader，有可能收到别人的请求吗，实验中是可能的,会收到的一个大的term
	//if rf.status == STATUS_LEADER {
	//	log.Println("I am leader, but get AppendEnties",rf.Detail())
	//	log.Println("args.term",args.Term,"currentTerm",rf.currentTerm)
	//}
	// 心跳一定来自leader
	rf.heartbeatChan <- true

	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.rpcRuleForAllServer(args.Term)
	// 如果日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，则返回 false
	// len(rf.log) >= args.PrevLogIndex + 1，说明本地日志长度 >= leader日志长度
	if len(rf.log) >= args.PrevLogIndex + 1 && rf.log[args.PrevLogIndex].Term == args.PrevLogTerm {
		// 该规则是需要保证follower已经包含了leader在PrevLogIndex之前所有的日志了
		for i:=0;i<len(args.Entries);i++ {
			if args.PrevLogIndex+1+i < len(rf.log){
				if rf.log[args.PrevLogIndex+1+i] != args.Entries[i]{
					// index相同，但是纪元不同
					rf.log = rf.log[:args.PrevLogIndex+1+i]// 之前的还是相同的，再加上本条之后的
					rf.log = append(rf.log,args.Entries[i:]...)
					break
				}
			}else {
				// 本条目不存在
				rf.log = append(rf.log,args.Entries[i:]...)
				break
			}
		}
		if args.LeaderCommit > rf.commitIndex {
			rf.commitIndex = min(args.LeaderCommit,len(rf.log)-1)
		}
		// !!为了查明为什么日志错了
		rf.checkLog(args)
		//在回复给RPCs之前需要更新到持久化存储之上
		rf.persist()
		reply.Term = rf.currentTerm
		reply.Success = true
		reply.NextIndex = len(rf.log)
		return
	}else {
		reply.Term = rf.currentTerm
		reply.Success = false
		reply.NextIndex = min(rf.commitIndex+1,args.PrevLogIndex-1)//
		return
	}
}

func (rf *Raft)incVoteCount() {
	rf.mu.Lock()
	rf.votedCount++
	rf.mu.Unlock()
}

func (rf *Raft)judgeCandidateResult() {
	if rf.status == STATUS_CANDIDATE && rf.votedCount > len(rf.peers) / 2 {
		rf.voteResultChan <- true
	}
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
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	// 将之前broadRequestVoteRPC的判断投票的结果放到此处
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	if ok {
		// 发送正常
		rf.mu.Lock()
		defer rf.mu.Unlock()
		rf.rpcRuleForAllServer(reply.Term)
		if reply.VoteGranted {
			rf.votedCount++
			rf.judgeCandidateResult()
		}
	}
	return ok
}

func (rf *Raft)updateCommitIndex() {
	// 如果存在一个N满足N>commitIndex,多数的matchIndex[i] >= N,并且 log[N].term == currentTerm:设置commitIndex = N
	// 找出 matchIndex（已经同步给各个follower的值）
	newCommitIndex := rf.commitIndex
	count := 0
	for _, nextIndex := range rf.nextIndex {
		if nextIndex-1 > rf.commitIndex {
			count++
			if newCommitIndex == rf.commitIndex || newCommitIndex > nextIndex-1 {
				newCommitIndex = nextIndex-1
			}
		}
	}
	//log.Println(count)
	if (count > len(rf.peers) / 2) && rf.status == STATUS_LEADER && rf.log[newCommitIndex].Term == rf.currentTerm{
		// 此时只能说 newCommitIndex 具备了能提交的可能性，但是还要保证 log[newCommitIndex].term == currentTerm 才能提交
		// 原因参见5.4.2
		//log.Println("leader:",rf.me,"update commitIndex to",newCommitIndex)
		rf.commitIndex = newCommitIndex
		rf.persist()
	}
}

func (rf *Raft) sendAppendEnties(server int, args AppendEntiesArgs, reply *AppendEntiesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEnties", args, reply)

	if ok {
		//
		if reply.Term > rf.currentTerm {
			rf.resetStateAndConvertToFollower(reply.Term)
		} else if reply.Success {
			//log.Println("leader:",rf.me,"sendAppendEnties to",server,"and get reply",reply.Success)
			// 更新nextInt和matchIndex
			rf.mu.Lock()
			defer rf.mu.Unlock()

			//rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
			//rf.matchIndex[server] = rf.nextIndex[server] - 1
			if reply.NextIndex > 0 {
				rf.nextIndex[server] = reply.NextIndex
			}
			rf.matchIndex[server] = rf.nextIndex[server] - 1
			if rf.nextIndex[server] < 1 {
				log.Fatal("rf.nextIndex[server]<1",rf.Detail())
			}
			rf.updateCommitIndex()
		} else {
			// 不认同PrevLogIndex，重传
			rf.nextIndex[server] = reply.NextIndex
			if reply.NextIndex > len(rf.log){
				log.Fatal("get big NextIndex from",server,"args",args,"reply",reply)
			}
			//if rf.nextIndex[server] > 1 {
			//	rf.nextIndex[server]--
			//}
		}
	}

	return ok
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

	if rf.status != STATUS_LEADER {
		isLeader = false
		return index, term, isLeader
	}

	rf.mu.Lock()
	index = len(rf.log)
	term = rf.currentTerm
	entry := Log{
		Term:    term,
		Command: command,
		Index:index,
	}
	// 1)Leader将该请求记录到自己的日志之中;
	rf.log = append(rf.log, entry)
	rf.nextIndex[rf.me] = index + 1
	rf.matchIndex[rf.me] = index
	rf.mu.Unlock()

	// 2)Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器
	go rf.broadcastAppendEntriesRPC() // 那在哪儿执行应用到状态机的操作呢？在stateMachine中
	//log.Println("broadcastAppendEntriesRPC")
	return index, term, isLeader
}

func (rf *Raft)broadcastAppendEntriesRPC() {
	// 发送消息给给各个peer
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_LEADER {
			// ！！！由于此处使用goroutine,有可能后面的请求先到server，即大的rf.commitIndex先到，反而前面的请求后到，需要在处理端处理
			go func(i int) {
				rf.mu.Lock()
				prevLogIndex := rf.nextIndex[i] - 1
				if prevLogIndex < 0 || prevLogIndex + 1 > len(rf.log) {
					log.Fatal("prevLogIndex < 0  || prevLogIndex + 1 > len(rf.log)",rf.Detail())
				}
				args := AppendEntiesArgs{
					Term:         rf.currentTerm,
					LeaderId:     rf.me,
					LeaderCommit: rf.commitIndex,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:rf.log[prevLogIndex].Term,
					Entries:rf.log[prevLogIndex + 1:], // 如果没有新日志，自然而然是空
				}
				rf.mu.Unlock()
				reply := &AppendEntiesReply{}
				rf.sendAppendEnties(i, args, reply)
			}(i)
		}
	}
}


//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}
func (rf *Raft) resetElectionTimeout() time.Duration {
	//[electiontimeout, 2 * electiontimeout - 1]
	rand.Seed(time.Now().UTC().UnixNano())
	rf.randomizedElectionTimeout = rf.electionTimeout + time.Duration(rand.Int63n(rf.electionTimeout.Nanoseconds()))
	return rf.randomizedElectionTimeout
}
func (rf *Raft) convertToFollower() {
	rf.mu.Lock()
	rf.status = STATUS_FOLLOWER
	rf.mu.Unlock()
}

func (rf *Raft) convertToCandidate() {
	rf.mu.Lock()
	rf.status = STATUS_CANDIDATE
	rf.mu.Unlock()
}

func (rf *Raft)convertToLeaderAndInitState() {
	rf.mu.Lock()
	rf.status = STATUS_LEADER
	maxIndex := len(rf.log)
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = maxIndex
		rf.matchIndex[i] = 0
	}
	rf.mu.Unlock()
	//log.Println(rf.nextIndex)
}

func (rf *Raft) convertToLeader() {
	rf.mu.Lock()
	rf.status = STATUS_LEADER
	rf.mu.Unlock()
}

func (rf *Raft) resetStateAndConvertToFollower(term int) {
	rf.mu.Lock()
	rf.currentTerm = term
	rf.status = STATUS_FOLLOWER
	rf.votedFor = -1
	rf.mu.Unlock()
}

// 广播投票信息
func (rf *Raft) broadcastRequestVoteRPC() {
	// 启动goroutine开始
	rf.mu.Lock()
	defer rf.mu.Unlock()
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: len(rf.log) - 1,
		LastLogTerm:  rf.log[len(rf.log) - 1].Term,
	}
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_CANDIDATE {
			go func(index int, args RequestVoteArgs) {
				if rf.status != STATUS_CANDIDATE {
					return
				}
				reply := &RequestVoteReply{}
				//log.Println(fmt.Sprintf("candidate:%d sendRequestVote to %d",rf.me,index))
				rf.sendRequestVote(index, args, reply)
			}(i,args)
		}
	}
}

// 广播心跳
//func (rf *Raft) broadcastHeartbeat() {
//	lastLogIndex := len(rf.log) - 1
//	// If last log index ≥ nextIndex for a follower: send AppendEntries RPC with log entries starting at nextIndex
//	for i := range rf.peers {
//		if i != rf.me && rf.status == STATUS_LEADER {
//			args := AppendEntiesArgs{
//				Term:         rf.currentTerm,
//				LeaderId:     rf.me,
//				LeaderCommit: rf.commitIndex,
//			}
//			// 判断log的最大index ≥ nextIndex,如果大于，需要发送从nextIndex开始的log
//			if lastLogIndex >= rf.nextIndex[i] {
//				args.Entries = rf.log[rf.nextIndex[i]:lastLogIndex]
//			}
//			//log.Println("rf.nextIndex[i]",rf.nextIndex[i])
//			reply := &AppendEntiesReply{}
//			//log.Println("leader:", args.LeaderId, "send heartbeat to follower:", i)
//			ok := rf.sendAppendEnties(i, args, reply)
//			if ok {
//				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
//				if reply.Term > rf.currentTerm {
//					log.Println("LeaderId:", args.LeaderId, "has small term:", args.Term, "than follower:", i, "currentTerm:", reply.Term)
//					log.Println("LeaderId:", args.LeaderId, "convert to follower")
//					rf.resetStateAndConvertToFollower(reply.Term)
//					break
//				}
//				//log.Println("sendAppendEnties reply success:",reply.Success)
//				if reply.Success {
//					//If successful: update nextIndex and matchIndex for follower
//					rf.nextIndex[i] = lastLogIndex + 1
//					rf.matchIndex[i] = lastLogIndex
//				} else {
//					// If AppendEntries fails because of log inconsistency:decrement nextIndex and retry
//					rf.nextIndex[i]--
//				}
//			} else {
//				// rpc失败，不断重试？
//				//log.Println("sendAppendEnties to follower:",i,"error")
//			}
//		}
//	}
//}

// 成为leader，开始心跳


func (rf *Raft) leader() {
	// 外层loop
	time.Sleep(rf.heartbeatTimeout)
	go rf.broadcastAppendEntriesRPC()
}

func (rf *Raft)resetCandidateState() {
	rf.mu.Lock()
	rf.currentTerm++
	rf.votedFor = rf.me
	rf.votedCount = 1
	rf.mu.Unlock()
}

// 选举行为
func (rf *Raft) candidate() {
	// 外层有forloop,因此此处没必要loop

	// 清空几个数据
	select {
	case <-rf.voteResultChan:
	case <-rf.heartbeatChan:
		// 退出选举,转变为follower
		rf.status = STATUS_FOLLOWER
		return
	default:
	}

	// 第一步，新增本地任期和投票
	rf.resetCandidateState()
	// 第二步开始广播投票
	go rf.broadcastRequestVoteRPC()

	// 第三步等待结果
	// 保持candidate状态，直到下面3种情况发生
	// 1)他自己赢得了选举;
	// 2)收到AppendEntries得知另外一个服务器确立他为Leader，转变为follower
	// 3)一个周期时间过去但是没有任何人赢得选举，开始新的选举
	select {
	case becomeLeader := <-rf.voteResultChan:
	// 此处voteResultChan有两个地方会写，一个是获得大多数票
		if becomeLeader {
			//他自己赢得了选举,发送appendEntries给
			rf.convertToLeaderAndInitState()
			go rf.broadcastAppendEntriesRPC()
		}
	case <-time.After(rf.resetElectionTimeout()):
	// 开始新的选举
	case <-rf.heartbeatChan:
	// 收到别的服务器已经转变为leader的通知，退出选举,转变为follower
		rf.status = STATUS_FOLLOWER
	}
}

func (rf *Raft)follower() {
	// 等待心跳，如果心跳未到，但是选举超时了，则开始新一轮选举
	select {
	case <-rf.heartbeatChan:
	case <-time.After(rf.resetElectionTimeout()):
		// 开始重新选举
		rf.status = STATUS_CANDIDATE
	}
}

func (rf *Raft) loop() {
	for {
		switch rf.status {
		case STATUS_FOLLOWER:
			rf.follower()
		case STATUS_CANDIDATE: // 开始新选举
			//log.Println("now I begin to candidate,index:", rf.me)
			rf.candidate()
		case STATUS_LEADER:
			//log.Println("now I am the leader,index:", rf.me)
			rf.leader()
		}
	}
}
// 此处好的方法是stateMachine和其他通过channel通道
func (rf *Raft)stateMachine(applyCh chan ApplyMsg) {
	// 在此运用已经agree的日志到状态机
	for {
		// ！！！！此处必须要有一个sleep，不然相当于此goroutine一直占有cpu，不会放弃
		time.Sleep(50 * time.Millisecond)
		// 如果commitIndex > lastApplied，那么就 lastApplied 加一，并把log[lastApplied]应用到状态机中

		if rf.commitIndex > rf.lastApplied {
			if len(rf.log) < rf.commitIndex + 1 {
				// 这种情况出现时不正常的，因为commitIndex应该是leader发过来的，最大不会超过rf.log
				// 为什么会出现这个，大家想下，follower中如果commitIndex是server发过来的，可能出现commitIndex大，但是log还没有发送过来的情况
				log.Fatal("machine!! ", rf.Detail())
			}
			commitIndex := rf.commitIndex
			lastApplied := rf.lastApplied
			rf.lastApplied = commitIndex
			for i := lastApplied + 1; i <= commitIndex; i++ {
				applymsg := ApplyMsg{
					Index:   rf.log[i].Index,
					Command: rf.log[i].Command,
				}
				applyCh <- applymsg // 此处可能阻塞
			}
			// 应用完状态机后，更新rf.lastApplied
		}
	}
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

	rf.heartbeatChan = make(chan bool)
	rf.voteResultChan = make(chan bool)
	rf.votedFor = -1
	rf.currentTerm = 0
	rf.log = []Log{{Term: rf.currentTerm, Index: 0}}
	rf.status = STATUS_FOLLOWER
	rf.electionTimeout = 1000 * time.Millisecond // 1000ms
	rf.heartbeatTimeout = 50 * time.Millisecond
	cnt := len(rf.peers)
	rf.nextIndex = make([]int, cnt)
	rf.matchIndex = make([]int, cnt)

	// Your initialization code here.
	// 刚开始启动所有状态都是 follower，然后启动 election timeout 和 heartbeat timeout

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	// check index
	rf.checkLog(1)

	for i :=0; i < cnt; i++ {
		rf.nextIndex[i] = len(rf.log)
	}

	go rf.loop()
	go rf.stateMachine(applyCh)

	return rf
}

func (rf *Raft)checkLog(v interface{}) {
	for i:=0;i<len(rf.log);i++ {
		if i != rf.log[i].Index {
			log.Println("check log type:",reflect.TypeOf(v).String(),"value:",v)
			log.Fatal("error log index",rf.Detail())
		}
	}
}

