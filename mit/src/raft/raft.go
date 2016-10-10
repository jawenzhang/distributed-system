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
}

type Log struct {
	Term    int         // 任期,收到命令时，当前server的任期
	Command interface{} // 命令
	index   int         // 在数组中的位置
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
//
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	// 收到投票请求，判断candidate的任期是否大于自己的任期，是则更新自己的任期
	// 检查当前任期是否已经投票了，是则告知已投
	// 检查candidate的日志是不是比自己的日志新
	// 1. Reply false if term < currentTerm
	log.Println("peer:", rf.me, "deal RequestVote from candidate:", args.CandidateId)

	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
	} else if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		reply.VoteGranted = false
	} else if args.LastLogIndex < rf.commitIndex || args.LastLogTerm < rf.currentTerm {
		//检查candidate的日志是不是比自己的日志新
		reply.VoteGranted = false
	} else {
		reply.VoteGranted = true
		if args.Term > rf.currentTerm {
			rf.currentTerm = args.Term
		}
	}
	reply.Term = rf.currentTerm
	log.Println("deal RequestVote done, voteGranted:", reply.VoteGranted)
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) AppendEnties(args AppendEntiesArgs, reply *AppendEntiesReply) {
	// Your code here.
	// 如果是宣称leader的请求，判断任期是否小于，小于的都丢弃
	// 如果等于或者大于则承认身份，并将状态改为follower
	reply.Term = rf.currentTerm

	// 小于的都丢弃
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}
	rf.heartbeatChan <- true // 不应该阻塞，chan有1
	if len(args.Entries) == 0 {
		// 宣称是leader的
		rf.mu.Lock()
		if args.Term > rf.currentTerm {
			rf.currentTerm = args.Term
		}
		rf.status = STATUS_FOLLOWER
		rf.mu.Unlock()
		reply.Success = true
		return
	}
	//TODO 其他请求，暂时都忽略
	return

	// 如果是日志复制，检查prevLogIndex,prevLogTerm，如果找不到，则拒绝新的log
	//1. Reply false if term < currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
	} else if (len(rf.log)) < args.PrevLogIndex || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		// 2. Reply false if log doesn’t contain an entry at prevLogIndex
		// whose term matches prevLogTerm
		return
	} else {
		// 3. If an existing entry conflicts with a new one (same index
		// but different terms), delete the existing entry and all that
		// follow it

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
// 函数需要立即返回，但是不保证这个命令已经持久化到了日志中
// 第一个返回值表示如果命令提交，下标将会是多少
// 第二个返回值是当前的任期
// 第三个返回值表明当前server是否是leader
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// 客户端的一次日志请求操作触发
	// 1)Leader将该请求记录到自己的日志之中;
	// 2)Leader将请求的日志以并发的形式,发送AppendEntries RCPs给所有的服务器;
	// 3)Leader等待获取多数服务器的成功回应之后(如果总共5台,那么只要收到另外两台回应),
	// 将该请求的命令应用到状态机(也就是提交),更新自己的commitIndex 和 lastApplied值;
	// 4)Leader在与Follower的下一个AppendEntries RPCs通讯中,
	// 就会使用更新后的commitIndex,Follower使用该值更新自己的commitIndex;
	// 5)Follower发现自己的 commitIndex > lastApplied
	// 则将日志commitIndex的条目应用到自己的状态机(这里就是Follower提交条目的时机)
	return index, term, isLeader
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
	rand.Seed(time.Now().UTC().UnixNano())
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
			log.Println("candidate:", args.CandidateId, "send RequestVote to peer:", i)
			ok := rf.sendRequestVote(i, args, reply)
			if ok {
				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
				if reply.Term > rf.currentTerm {
					rf.convertToFollower()
					break
				} else if reply.VoteGranted {
					rf.votedCount++
				} else {
					// do nothing
				}
			} else {
				// rpc失败，不断重试？
			}
		}
	}
	if rf.votedCount > int(len(rf.peers) / 2) {
		// 选举成功，否则失败
		rf.voteResultChan <- true
	} else {
		rf.voteResultChan <- false
	}
}
// 广播心跳
func (rf *Raft)broadcastHeartbeat() {
	for i := range rf.peers {
		if i != rf.me && rf.status == STATUS_LEADER {
			args := AppendEntiesArgs{
				Term:rf.currentTerm,
				LeaderId:rf.me,
				//prevLogIndex:rf.commitIndex,
				//prevLogTerm:rf.log[rf.commitIndex].Term,
			}
			reply := &AppendEntiesReply{}
			log.Println("leader:", args.LeaderId, "send heartbeat to follower:", i)
			ok := rf.sendAppendEnties(i, args, reply)
			if ok {
				// 判断任期是否大于自己，如果大于则转换为follower,退出循环
				if reply.Term > rf.currentTerm {
					rf.convertToFollower()
					break
				}
			} else {
				// rpc失败，不断重试？
			}
		}
	}
}

// 成为leader，开始心跳
func (rf *Raft)leader() {
	ticker := time.Tick(rf.heartbeatTimeout)

	for {
		select {
		case <-ticker:
		// 发送心跳
			log.Println("leader:",rf.me,"begin to broadcastHeartbeat")
			go func() {
				rf.broadcastHeartbeat()
			}()
		}
	}
}

// 选举行为
func (rf *Raft)candidate() {
	var contiue = true
	for {

		select {
		case <-rf.heartbeatChan:
			contiue = false
		default:
		}
		if !contiue {
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
				// 选举失败，查看是否收到
			}
		case <-time.After(rf.resetElectionTimeout()):
		// 重新开始选举
			go func() {
				<-rf.voteResultChan
			}()
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
			log.Println("now I am follower,index:", rf.me, "start election timeout:", rf.resetElectionTimeout())
			// 等待心跳，如果心跳未到，但是选举超时了，则开始新一轮选举
				select {
				case <-rf.heartbeatChan:
				case <-time.After(rf.randomizedElectionTimeout):
				// 开始重新选举
					log.Println("election timeout:", rf.randomizedElectionTimeout)
					if rf.status != STATUS_FOLLOWER {
						// panic
						log.Fatal("status not right when in follower and after randomizedElectionTimeout:", rf.randomizedElectionTimeout)
					}
					rf.convertToCandidate()
				}
		case STATUS_CANDIDATE:// 开始新选举
			log.Println("now I begin to candidate,index:", rf.me)
			rf.candidate()
		case STATUS_LEADER:
			log.Println("now I am the leader,index:", rf.me)
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

	rf.heartbeatChan = make(chan bool, 1)
	rf.voteResultChan = make(chan bool)
	rf.votedFor = -1
	rf.log = []Log{{Term:0, index:0}}
	rf.status = STATUS_FOLLOWER
	rf.electionTimeout = 1000 * time.Millisecond // 1000ms
	rf.heartbeatTimeout = 500 * time.Millisecond
	// Your initialization code here.
	// 刚开始启动所有状态都是 follower，然后启动 election timeout 和 heartbeat timeout


	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.loop()

	return rf
}
