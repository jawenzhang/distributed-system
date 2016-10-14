package raft

// 所有server的原则 Rules for Servers
// 1. 如果commitIndex > lastApplied:则递增lastApplied,应用 log[lastApplied] 到状态机之中
// 2. 如果Rpc请求或回复包括纪元T > currentTerm: 设置currentTerm = T,转换成 follower, 并且设置 votedFor=-1，表示未投票

// rules for Followers
// 只接受 candidates与leaders的RPC请求
// 如果选举超时时间达到,并且没有收到来自当前leader或者要求投票的候选者的 AppendEnties RPC调 :转换角色为candidate

// rules for Candidates
// 转换成candidate时,开始一个选举:
// 1. 递增currentTerm;投票给自己;
// 2. 重置election timer;
// 3. 向所有的服务器发送 RequestVote RPC请求
// 如果获取服务器中多数投票:转换成Leader
// 如果收到从新Leader发送的AppendEnties RPC请求:转换成follower
// 如果选举超时时间达到:开始一次新的选举

// rules for Leaders
// 给每个服务器发送初始空的AppendEntires RPCs(heartbeat);指定空闲时间之 后重复该操作以防 election timeouts
// 如果收到来自客户端的命令:将条目插入到本地日志,在条目应用到状态机后回复给客户端
// 如果last log index >= nextIndex for a follower:发送包含开始于nextIndex的日志条目的AppendEnties RPC
// 如果成功:为follower更新nextIndex与matchIndex
// 如果失败是由于日志不一致:递减nextIndex然后重试
// 如果存在以个N满足 N>commitIndex,多数的matchIndex[i] >= N,并且 log[N].term == currentTerm:设置commitIndex = N

// AppendEntries RPC的实现：在回复给RPCs之前需要更新到持久化存储之上
// 有3类用途
// 1. candidate赢得选举的后，宣誓主权
// 2. 保持心跳
// 3. 让follower的日志和自己保持一致
// 接收者的处理逻辑：
// 1. 如果term < currentTerm 则返回false
// 2. 如果日志不包含一个在preLogIndex位置纪元为prevLogTerm的条目,则返回 false
//		该规则是需要保证follower已经包含了leader在PrevLogIndex之前所有的日志了
// 3. 如果一个已存在的条目与新条目冲突(同样的索引但是不同的纪元),则删除现存的该条目与其后的所有条
// 4. 将不在log中的新条目添加到日志之中
// 5. 如果leaderCommit > commitIndex,那么设置 commitIndex = min(leaderCommit,index of last new entry)

// RequestVote RPC 的实现: 由候选者发起用于收集选票
// 1. 如果term < currentTerm 则返回false
// 2. 如果本地的voteFor为空或者为candidateId,
// 		并且候选者的日志至少与接受者的日志一样新,则投给其选票
// 怎么定义日志新
// 比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新
// 如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新
// 如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。

// 以上所有的规则保证的下面的几个点：
// 1. Election Safety 在一个特定的纪元中最多只有一个Leader会被选举出来
// 2. Leader Append-Only Leader不会在他的日志中覆盖或删除条 ,他只执行添加新的条
// 3. Log Matching:如果两个日志包含了同样index和term的条 ,那么在该index之前的所有条目都是相同的
// 4. Leader Completeness:如果在一个特定的term上提交了一个日志条目,那么该条目将显示在编号较大的纪元的Leader的日志里
// 5. State Machine Safety:如果一个服务器在一个给定的index下应用一个日志条目到他的状态机上,没有其他服务器会在相同index上应用不同的日志条目




