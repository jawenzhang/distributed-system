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

import "sync"
import (
	"labrpc"
	"time"
	"math/rand"
	"log"
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
	applyCh                   chan ApplyMsg
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
type AppendEntiesArgs struct {
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

//
// 如果收到一个大于自己任期的投票请求，怎么处理？需要将自己的votedFor更新嘛？
// 原则：If RPC request or response contains term T > currentTerm: set currentTerm = T, convert to follower
// 如果收到的请求或者响应中，包含的term大于当前的currentTerm，设置currentTerm=term，然后变为follower
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {

	// 收到投票请求，判断candidate的任期是否大于自己的任期，是则更新自己的任期
	// 检查当前任期是否已经投票了，是则告知已投
	// 检查candidate的日志是不是比自己的日志新
	// 1. Reply false if term < currentTerm
	//log.Println("peer:", rf.me, "deal RequestVote from candidate:", args.CandidateId)

	//select {// 防止已经收到心跳了，写阻塞
	//case rf.heartbeatChan <- true:
	//default:
	//}

	reply.Term = rf.currentTerm
	currentTerm := rf.currentTerm

	if args.Term > currentTerm {
		// 大于则直接转换为follower，并更新当前的currentTerm和voteFor
		//log.Println("candidateId:", args.CandidateId, "has big term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm, "status:", rf.status)
		rf.resetStateAndConvertToFollower(args.Term)
	}
	// granting vote to candidate,类似收到心跳，会重置election timeout
	//log.Println("heartbeatChan len:", len(rf.heartbeatChan))
	rf.heartbeatChan <- true // 目的了为了取消在follower状态下等待选举超时,但是当问题是在实现leader任务的时候，没有一个点触出发退出心跳

	if args.Term < currentTerm {
		// 过时的请求
		//log.Println("previous term:", args.Term, "currentTerm:", currentTerm)
		reply.VoteGranted = false
	} else if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		//log.Println("follower:", rf.me, "has votedFor:", rf.votedFor)
		reply.VoteGranted = false
	} else if rf.commitIndex != 0 && (args.LastLogIndex < rf.commitIndex || args.LastLogTerm < currentTerm) {
		// rf.commitIndex == 0 表示还没有日志，则没必要检查
		// 检查candidate的日志是不是比自己的日志新
		//log.Println("candidate:", args.CandidateId, "'slog is not at least as up-to-date as receiver’s log")
		reply.VoteGranted = false
	} else {

		rf.mu.Lock()
		rf.votedFor = args.CandidateId
		rf.mu.Unlock()

		reply.VoteGranted = true
	}
	//log.Println("deal RequestVote done, voteGranted:", reply.VoteGranted)
}

func (rf *Raft)checkCommitIndexAndApplied() {
	if rf.commitIndex > rf.lastApplied {
		commitIndex := rf.commitIndex
		lastApplied := rf.lastApplied
		// TODO:需要应用到状态机
		rf.mu.Lock()
		rf.lastApplied = rf.commitIndex
		rf.mu.Unlock()
		for i:=lastApplied+1; i<=commitIndex;i++ {
			applymsg := ApplyMsg{
				Index:rf.log[i].Index,
				Command:rf.log[i].Command,
			}
			rf.applyCh <- applymsg // 可能阻塞？
		}
	}
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) AppendEnties(args AppendEntiesArgs, reply *AppendEntiesReply) {

	// 如果是宣称leader的请求，判断任期是否小于，小于的都丢弃
	// 如果等于或者大于则承认身份，并将状态改为follower

	reply.Term = rf.currentTerm
	currentTerm := rf.currentTerm
	// 小于的都丢弃
	if args.Term < currentTerm {
		log.Println("LeaderId:", args.LeaderId, "has small term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm)
		reply.Success = false
		return
	}
	// 立即转变为follower
	if args.Term > currentTerm {
		log.Println("LeaderId:", args.LeaderId, "has big term:", args.Term, "than follower:", rf.me, "currentTerm:", currentTerm)
		rf.resetStateAndConvertToFollower(args.Term)
	}

	rf.heartbeatChan <- true // 不应该阻塞，chan有1,心跳

	// 更新commitIndex
	rf.mu.Lock()
	if args.LeaderCommit > rf.commitIndex {
		// 更新自身的commitIndex，但是在哪去更新lastApplied呢？在follower中需要一个单独的 goroutine ，去更新lastApplied
		rf.commitIndex = args.LeaderCommit
		go rf.checkCommitIndexAndApplied()
	}
	rf.mu.Unlock()

	// 如果是heartbeat，返回
	if len(args.Entries) == 0 {
		// 宣称是leader的
		reply.Success = true
		return
	}

	// 如果是日志复制，检查prevLogIndex,prevLogTerm，如果找不到，则拒绝新的log
	if (len(rf.log)) < args.PrevLogIndex || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		// 2. Reply false if log doesn’t contain an entry at prevLogIndex
		// whose term matches prevLogTerm
		reply.Success = false
		return
	} else {

		firstEntry := args.Entries[0]
		//log.Println(args)
		if len(rf.log) > firstEntry.Index {
			// 3. If an existing entry conflicts with a new one (same index
			// but different terms), delete the existing entry and all that
			// follow it
			//log.Println("firstEntry.index:",firstEntry.Index)
			// 此处发送过来了0，为什么呢？去看
			rf.log = rf.log[:(firstEntry.Index - 1)]
		}
		rf.log = append(rf.log, args.Entries...)
		reply.Success = true
		return
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
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEnties(server int, args AppendEntiesArgs, reply *AppendEntiesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEnties", args, reply)
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

	// 客户端的一次日志请求操作触发
	go func(command interface{}) {
		// 异步执行
		// 1)Leader将该请求记录到自己的日志之中;
		entry := Log{
			Term:rf.currentTerm,
			Command:command,
			Index:len(rf.log),
		}
		rf.log = append(rf.log, entry)
		// 2)Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器
		agree := rf.broadcastAppendEntries([]Log{entry})
		// 3)Leader等待获取多数服务器的成功回应之后(如果总共5台,那么只要收到另外两台回应),
		// 将该请求的命令应用到状态机(也就是提交),更新自己的 commitIndex 和 lastApplied 值;

		//log.Println("leader:",rf.me,"broadcastAppendEntries and get:",agree)

		if agree {
			// TODO:应用到状态机
			rf.mu.Lock()
			rf.commitIndex++
			rf.lastApplied = rf.commitIndex
			rf.nextIndex[rf.me] = len(rf.log)
			rf.mu.Unlock()
			applymsg := ApplyMsg{
				Index:entry.Index,
				Command:entry.Command,
			}
			rf.applyCh <- applymsg // 可能阻塞？
		} else {
			// TODO:失败了怎么办?
		}
		// 4)Leader在与Follower的下一个AppendEntries RPCs通讯中,
		// 就会使用更新后的commitIndex,Follower使用该值更新自己的commitIndex;
		// 5)Follower发现自己的 commitIndex > lastApplied
		// 则将日志commitIndex的条目应用到自己的状态机(这里就是Follower提交条目的时机)

	}(command)

	index = len(rf.log)
	term = rf.currentTerm
	return index, term, isLeader
}

// Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器
func (rf *Raft)broadcastAppendEntries(entries []Log) bool {
	prevLog := rf.log[entries[0].Index - 1]
	args := AppendEntiesArgs{
		Term:rf.currentTerm,
		LeaderId:rf.me,
		LeaderCommit:rf.commitIndex,
		PrevLogIndex:prevLog.Index,
		PrevLogTerm:prevLog.Term,
		Entries:entries,
	}
	//log.Println(args)
	var successCnt int = 1
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_LEADER {
			// 日志的流向是  leader -> follower
			reply := &AppendEntiesReply{}
			// TODO:优化请求为并发
			ok := rf.sendAppendEnties(i, args, reply)
			if ok {
				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
				if reply.Term > rf.currentTerm {
					rf.resetStateAndConvertToFollower(reply.Term)
					break
				}
				if reply.Success {
					successCnt++
				} else {
					//TODO:接着发送小的index，直到匹配上
				}
			} else {
				// 发送失败怎么办？
			}
		}
	}
	if successCnt > len(rf.peers) / 2 {
		// 大多数都收到
		return true
	} else {
		return false
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
func (rf *Raft)resetElectionTimeout() time.Duration {
	//[electiontimeout, 2 * electiontimeout - 1]
	//rand.Seed(time.Now().UTC().UnixNano())
	rf.randomizedElectionTimeout = rf.electionTimeout + time.Duration(rand.Int63n(rf.electionTimeout.Nanoseconds()))
	return rf.randomizedElectionTimeout
}
func (rf *Raft)convertToFollower() {
	rf.mu.Lock()
	rf.status = STATUS_FOLLOWER
	rf.mu.Unlock()
}

func (rf *Raft)convertToCandidate() {
	rf.mu.Lock()
	rf.status = STATUS_CANDIDATE
	rf.mu.Unlock()
}

func (rf *Raft)convertToLeader() {
	rf.mu.Lock()
	rf.status = STATUS_LEADER
	rf.mu.Unlock()
}

func (rf *Raft)resetStateAndConvertToFollower(term int) {
	rf.mu.Lock()
	rf.currentTerm = term
	rf.status = STATUS_FOLLOWER
	rf.votedFor = -1
	rf.mu.Unlock()
}

// 广播投票信息
func (rf *Raft)broadcastRequestVoteRPC() {
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_CANDIDATE {
			args := RequestVoteArgs{
				Term:rf.currentTerm,
				CandidateId:rf.me,
				LastLogIndex:rf.commitIndex,
				LastLogTerm:rf.log[rf.commitIndex].Term,
			}
			reply := &RequestVoteReply{}
			//log.Println("candidate:", args.CandidateId, "send RequestVote to peer:", i)
			ok := rf.sendRequestVote(i, args, reply)
			if ok {
				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
				if reply.Term > rf.currentTerm {
					rf.resetStateAndConvertToFollower(reply.Term)
					break
				} else if reply.VoteGranted {
					rf.votedCount++
				} else {
					// 没投票给自己
				}
			} else {
				// rpc失败，不断重试？
			}
		}
	}
	if rf.votedCount > len(rf.peers) / 2 {
		// 选举成功，否则失败
		rf.voteResultChan <- true
	}
	//} else {
	//	rf.voteResultChan <- false
	//}
}
// 广播心跳
func (rf *Raft)broadcastHeartbeat() {
	lastLogIndex := len(rf.log) - 1
	// If last log index ≥ nextIndex for a follower: send AppendEntries RPC with log entries starting at nextIndex
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_LEADER {
			args := AppendEntiesArgs{
				Term:rf.currentTerm,
				LeaderId:rf.me,
				LeaderCommit:rf.commitIndex,
			}
			// 判断log的最大index ≥ nextIndex,如果大于，需要发送从nextIndex开始的log
			if lastLogIndex >= rf.nextIndex[i] {
				args.Entries = rf.log[rf.nextIndex[i]:lastLogIndex]
			}
			//log.Println("rf.nextIndex[i]",rf.nextIndex[i])
			reply := &AppendEntiesReply{}
			//log.Println("leader:", args.LeaderId, "send heartbeat to follower:", i)
			ok := rf.sendAppendEnties(i, args, reply)
			if ok {
				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
				if reply.Term > rf.currentTerm {
					rf.resetStateAndConvertToFollower(reply.Term)
					break
				}
				//log.Println("sendAppendEnties reply success:",reply.Success)
				if reply.Success {
					//If successful: update nextIndex and matchIndex for follower
					rf.nextIndex[i] = lastLogIndex + 1
					rf.matchIndex[i] = lastLogIndex
				} else {
					// If AppendEntries fails because of log inconsistency:decrement nextIndex and retry
					rf.nextIndex[i]--
				}
			} else {
				// rpc失败，不断重试？
			}
		}
	}
}


// 成为leader，开始心跳
func (rf *Raft)leader() {
	// 重新初始化leader的数据

	maxIndex := len(rf.log)
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = maxIndex
		rf.matchIndex[i] = 0
	}

	ticker := time.Tick(rf.heartbeatTimeout)
	// 什么时候不是leader了退出
	for {
		if rf.status != STATUS_LEADER {
			break
		}
		select {
		case <-ticker:
		// 发送心跳
		//	log.Println("leader:", rf.me, "begin to broadcastHeartbeat")
			go rf.broadcastHeartbeat()
		}
	}
}

// 选举行为
func (rf *Raft)candidate() {
	var contiue = true
	for {
		// 已经有新的leader产生了，此处再检查一次是防止在下面select种没选择到
		select {
		case <-rf.heartbeatChan:
			contiue = false
		default:
		}
		if !contiue {
			break
		}
		// 不是候选人了，退出选举
		if rf.status != STATUS_CANDIDATE {
			break
		}

		// 第一步，新增本地任期和投票
		rf.mu.Lock()
		rf.currentTerm++
		rf.votedFor = rf.me
		rf.votedCount = 1
		rf.mu.Unlock()

		// 第二步重置 election timer 并开始广播
		go rf.broadcastRequestVoteRPC()

		// 第三步等待结果
		// 保持candidate状态，直到下面3种情况发生
		// 1)他自己赢得了选举;
		// 2)收到AppendEntries得知另外一个服务器确立他为Leader，转变为follower
		// 3) 一个周期时间过去但是没有任何人赢得选举，开始新的选举
		select {
		case result := <-rf.voteResultChan:
			if result {
				//选举成功
				contiue = false
				rf.convertToLeader()
			} else {
				// 选举失败，应该等待超时，然后重新开始新一轮选举，而不是马上开始新一轮选举，这样子造成彼此都不成功
			}
		case <-rf.heartbeatChan:
			contiue = false
		case <-time.After(rf.resetElectionTimeout()):
		// 重新开始选举
		}

		if !contiue {
			break
		}
	}
}

func (rf *Raft) loop() {
	for {
		switch rf.status {
		case STATUS_FOLLOWER:
			rf.resetElectionTimeout()
			//log.Println("now I am follower,index:", rf.me, "start election timeout:", rf.randomizedElectionTimeout)
			// 等待心跳，如果心跳未到，但是选举超时了，则开始新一轮选举
				select {
				case <-rf.heartbeatChan:
				case <-time.After(rf.randomizedElectionTimeout):
				// 开始重新选举
				//	log.Println("follower:", rf.me, "election timeout:", rf.randomizedElectionTimeout)
					if rf.status != STATUS_FOLLOWER {
						// panic
						log.Fatal("status not right when in follower and after randomizedElectionTimeout:", rf.randomizedElectionTimeout)
					}
					rf.convertToCandidate()
				}
		case STATUS_CANDIDATE:// 开始新选举
			//log.Println("now I begin to candidate,index:", rf.me)
			rf.candidate()
		case STATUS_LEADER:
			//log.Println("now I am the leader,index:", rf.me)
			rf.leader()
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
	rf.applyCh = applyCh

	rf.heartbeatChan = make(chan bool, 1)
	rf.voteResultChan = make(chan bool)
	rf.votedFor = -1
	rf.currentTerm = 0
	rf.log = []Log{{Term:rf.currentTerm, Index:0}}
	rf.status = STATUS_FOLLOWER
	rf.electionTimeout = 1000 * time.Millisecond // 1000ms
	rf.heartbeatTimeout = 500 * time.Millisecond
	cnt := len(rf.peers)
	rf.nextIndex = make([]int, cnt)
	rf.matchIndex = make([]int, cnt)

	// Your initialization code here.
	// 刚开始启动所有状态都是 follower，然后启动 election timeout 和 heartbeat timeout


	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.loop()

	return rf
}
