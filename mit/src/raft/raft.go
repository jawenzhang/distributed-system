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
)

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
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
					 // Your data here.
	Term         int // candidate的任期
	CandidateId  int // candidate的id
	LastLogIndex int
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
type 	AppendEntiesArgs struct {
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
}

func (rf *Raft)reqMoreUpToDate(args *RequestVoteArgs) bool{
	// 比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新
	// 如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新
	// 如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。
	lastLog := rf.log[len(rf.log) - 1]
	rlastTerm := args.LastLogTerm
	rlastIndex := args.LastLogIndex
	//log.Println(lastLog,rlastIndex,rlastIndex)
	return rlastTerm > lastLog.Term || (rlastTerm == lastLog.Term && rlastIndex >= lastLog.Index )
}

//
// 如果收到一个大于自己任期的投票请求，怎么处理？需要将自己的votedFor更新嘛？
// 原则：If RPC request or response contains term T > currentTerm: set currentTerm = T, convert to follower
// 如果收到的请求或者响应中，包含的term大于当前的currentTerm，设置currentTerm=term，然后变为follower
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {

	// 对于是否投票需要考虑:
	// 1. candidate's term 足够新
	// 2. term要相等，本server还没有投票，或者投的就是candidate
	// 怎么定义日志新
	// 比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新
	// 如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新
	// 如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。

	currentTerm := rf.currentTerm

	if args.Term > currentTerm {
		// 大于则直接转换为follower，并更新当前的currentTerm和voteFor
		if rf.status != STATUS_FOLLOWER{
			//log.Println("candidateId:", args.CandidateId, "has big term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm, "status:", rf.status)
			rf.resetStateAndConvertToFollower(args.Term)
		}
	}
	reply.Term = rf.currentTerm

	if args.Term < currentTerm {
		//如果term < currentTerm返回 false
		//log.Println("previous term:", args.Term, "currentTerm:", currentTerm)
		reply.VoteGranted = false
	} else if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		//如果 votedFor 不为空也不是 candidateId，返回 false
		//log.Println("follower:", rf.me, "has votedFor:", rf.votedFor)
		reply.VoteGranted = false
	} else if !rf.reqMoreUpToDate(&args) {
		//log.Println("candidate:", args.CandidateId, "'slog is not at least as up-to-date as receiver’s log")
		reply.VoteGranted = false
	} else {
		rf.mu.Lock()
		rf.votedFor = args.CandidateId
		rf.mu.Unlock()
		reply.VoteGranted = true
	}
	//log.Println("follower:",rf.me,"deal RequestVote done, voteGranted:", reply.VoteGranted)
}


//
// example RequestVote RPC handler.
//
func (rf *Raft) AppendEnties(args AppendEntiesArgs, reply *AppendEntiesReply) {
	// TODO:是否需要判断是否是心跳？
	rf.heartbeatChan <- true

	//- 如果 term < currentTerm 就返回 false （5.1 节）
	//- 如果日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，则返回 false （5.3 节）
	//- 如果已经已经存在的日志条目和新的产生冲突（相同偏移量但是任期号不同），删除这一条和之后所有的 （5.3 节）
	//- 附加任何在已有的日志中不存在的条目
	//- 如果 leaderCommit > commitIndex，令 commitIndex 等于 leaderCommit 和 新日志条目索引值中较小的一个
	currentTerm := rf.currentTerm
	reply.Term = currentTerm
	// 立即转变为follower
	if args.Term > currentTerm {
		log.Println("LeaderId:", args.LeaderId, "has big term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm)
		rf.resetStateAndConvertToFollower(args.Term)
		return
	}
	// 如果 term < currentTerm 就返回 false
	if args.Term < currentTerm {
		reply.Success = false
		//log.Println("LeaderId:", args.LeaderId, "has small term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm)
		return
	}
	//如果日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，则返回 false
	if len(rf.log) > args.PrevLogIndex && rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		// 2. Reply false if log doesn’t contain an entry at prevLogIndex
		// whose term matches prevLogTerm
		reply.Success = false
		return
	}
	//if len(args.Entries) == 0{
	//	// 心跳
	//	reply.Success = true
	//	return
	//}
	// 如果已经已经存在的日志条目和新的产生冲突（相同偏移量但是任期号不同），删除这一条和之后所有的
	reply.Success = true
	if len(args.Entries) > 0 { // 当len(args.Entries) = 0 的时候，是心跳或者是确认leaderCommit的命令有两种情况
		firstEntry := args.Entries[0]
		//log.Println(args)

		rf.mu.Lock()
		if len(rf.log) > firstEntry.Index {
			rf.log = rf.log[:(firstEntry.Index - 1)] // 有旧的,则直接删除
		}
		rf.log = append(rf.log, args.Entries...)
		rf.mu.Unlock()
		//log.Println("update commitIndex","args.LeaderCommit",args.LeaderCommit," rf.commitIndex", rf.commitIndex)
	}
	if args.LeaderCommit > rf.commitIndex {
		// 如果 leaderCommit > commitIndex，令 commitIndex 等于 leaderCommit 和 新日志条目索引值中较小的一个
		// 更新自身的commitIndex，但是在哪去更新lastApplied呢？在follower中需要一个单独的 goroutine ，去更新lastApplied
		//log.Println("leader",args.LeaderId,"has big commit",args.LeaderCommit,"than me",rf.me,"commit",rf.commitIndex)
		rf.mu.Lock()
		lastIndex := len(rf.log) - 1
		if args.LeaderCommit > lastIndex {
			rf.commitIndex = lastIndex
		}else {
			rf.commitIndex = args.LeaderCommit
		}
		rf.mu.Unlock()
		//log.Println("after update,me",rf.me,"commit",rf.commitIndex)
		//go rf.checkCommitIndexAndApplied()
	}
	return
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
		if reply.VoteGranted && reply.Term == rf.currentTerm {
			// 满足同一任期
			// 投票了
			rf.incVoteCount()
			rf.judgeCandidateResult()
		} else if reply.Term > rf.currentTerm {
			// 转换为follower
			rf.resetStateAndConvertToFollower(reply.Term)
		}
	}
	return ok
}

func (rf *Raft)updateCommitIndex() {
	rf.mu.Lock()
	newCommitIndex := rf.commitIndex
	count := 0
	for _,logIndex := range rf.nextIndex {
		if logIndex-1 > rf.commitIndex {
			count++
			if newCommitIndex == rf.commitIndex || newCommitIndex > logIndex-1 {
				newCommitIndex = logIndex-1
			}
		}
	}
	//log.Println(count)
	if count > len(rf.peers)/2 && rf.status==STATUS_LEADER{
		rf.commitIndex = newCommitIndex
	}
	rf.mu.Unlock()
	//log.Println("leader",rf.me,"commitIndex",rf.commitIndex)
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
			rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
			rf.matchIndex[server] = rf.nextIndex[server] - 1
			rf.mu.Unlock()
			// 在此处更新commitIndex
			//log.Println(rf.nextIndex)
			rf.updateCommitIndex()
		} else {
			// 不认同PrevLogIndex，重传
			rf.mu.Lock()
			rf.nextIndex[server]--
			rf.mu.Unlock()
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
	index = len(rf.log)
	term = rf.currentTerm
	entry := Log{
		Term:    term,
		Command: command,
		Index:index,
	}
	rf.mu.Lock()
	// 1)Leader将该请求记录到自己的日志之中;
	rf.log = append(rf.log, entry)
	rf.nextIndex[rf.me] = index+1
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
			go func(i int) {
				prevLogIndex := rf.nextIndex[i] - 1
				//log.Println(prevLogIndex,rf.nextIndex)
				args := AppendEntiesArgs{
					Term:         rf.currentTerm,
					LeaderId:     rf.me,
					LeaderCommit: rf.commitIndex,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:rf.log[prevLogIndex].Term,
					Entries:rf.log[prevLogIndex + 1:], // 如果没有新日志，自然而然是空
				}
				reply := &AppendEntiesReply{}
				//if len(args.Entries) > 0 {
					//log.Println(fmt.Sprintf("leader:%d send LeaderCommit:%d,PrevLogIndex:%d,PrevLogTerm:%d to follower:%d",
					//				args.LeaderId,args.LeaderCommit,args.PrevLogIndex,args.PrevLogTerm,i))
				//}
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
	rf.mu.Unlock()
	maxIndex := len(rf.log)
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = maxIndex
		rf.matchIndex[i] = 0
	}
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
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: len(rf.log) - 1,
		LastLogTerm:  rf.log[len(rf.log) - 1].Term,
	}
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_CANDIDATE {
			go func(index int) {
				reply := &RequestVoteReply{}
				//log.Println(fmt.Sprintf("candidate:%d sendRequestVote to %d",rf.me,index))
				rf.sendRequestVote(index, args, reply)
				// TODO：deal sendRequestVote error
			}(i)
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
		return // 退出选举
	default:
	}

	// 第一步，新增本地任期和投票
	rf.resetCandidateState()
	// 第二步开始广播投票
	//log.Println(fmt.Sprintf("candidate:%d begin to broadcastRequestVoteRPC",rf.me))
	rf.broadcastRequestVoteRPC()

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
	// 收到别的服务器已经转变为leader的通知，退出
	}
}

func (rf *Raft)follower() {
	// 外层有forloop,因此此处没必要loop
	rf.resetElectionTimeout()
	//log.Println("now I am follower,index:", rf.me, "start election timeout:", rf.randomizedElectionTimeout)
	// 等待心跳，如果心跳未到，但是选举超时了，则开始新一轮选举
	select {
	case <-rf.heartbeatChan:
	case <-time.After(rf.randomizedElectionTimeout):
	//  开始重新选举
	//	log.Println("follower:", rf.me, "election timeout:", rf.randomizedElectionTimeout)
		if rf.status != STATUS_FOLLOWER {
			log.Fatal("status not right when in follower and after randomizedElectionTimeout:", rf.randomizedElectionTimeout)
		}
		rf.convertToCandidate()
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


func (rf *Raft)stateMachine(applyCh chan ApplyMsg) {
	// 在此运用已经agree的日志到状态机
	for {
		// ！！！！此处必须要有一个sleep，不然相当于此goroutine一直占有cpu，不会放弃
		time.Sleep(50*time.Millisecond)
		// 如果commitIndex > lastApplied，那么就 lastApplied 加一，并把log[lastApplied]应用到状态机中
		if rf.commitIndex > rf.lastApplied {
			go func() {
				//log.Println("server",rf.me,"is",rf.status,"(commitIndex,lastApplied)",rf.commitIndex,rf.lastApplied)
				// 应用到状态机
				rf.mu.Lock()
				commitIndex := rf.commitIndex
				lastApplied := rf.lastApplied
				rf.lastApplied = commitIndex
				rf.mu.Unlock()
				for i := lastApplied + 1; i <= commitIndex; i++ {
					applymsg := ApplyMsg{
						Index:   rf.log[i].Index,
						Command: rf.log[i].Command,
					}
					applyCh <- applymsg
				}
			}()
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

	go rf.loop()
	go rf.stateMachine(applyCh)

	return rf
}
