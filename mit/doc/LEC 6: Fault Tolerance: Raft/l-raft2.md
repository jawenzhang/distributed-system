注意：新的leader会强制让所有的follower的日志和自己保持一致，方法是通过AppendEntries prevLogTerm机制

# 一个新的leader能够回滚之前任期内已经执行的条目嘛？

例如：一个之前已经执行过的entry在新的leader的日志中丢失了

这会是一个灾难：违反了State Machine Safety

解决方法：Raft不会选举一个server为leader，如果它没有之前执行的entry



# 如何进行leader的选举？

**我们能简单的选择拥有最长日志的server嘛？**

例子：

```
	S1: 5 6 7
    S2: 5 8
    S3: 5 8
```

首先，我们看下上面的场景怎么发生的：

```
S1在term 6成为leader
crash+reboot
S1在term 7又成为leader
crash，stay down
S2在term 8成为leader，只有S2和S3 live
S2 crash
```

那谁会成为leader呢？

S1有最长的日志，但是entry 8已经执行了！！！

所以新的leader只能是S2或者S3

回答上面的问题：谁成为leader不能只判断谁的日志长？（日志长的考虑是尽可能持有最多的日志，信息最多）



在论文5.4.1中说选举规则是：**"at least as up to date"**，那怎么定义日志新呢？

1. 比较最后的entry，谁的term大谁赢，term大意味着在时间轴上靠后
2. term相同，比较谁的日志长

通过上面两点：**"at least as up to date"**保证了新的leader包含了所有可能已经执行的entry



# 怎么快速回滚？

图2规则中每个rpc都退一个entry太慢了！

```
  S1: 4 4 4 4
  S2: 4 4 5 5 6 6
  S3: 4 4 5 5
```

S3当选了term7的leader，如果follower拒绝了，那回复中包含：

不一致entry的任期（term）

不一致任期的第一个entry的下标

如果leader知道不一致的任期：

​	将nextIndex[i]设置为不一致任期的最后一个entry

否则：

​	将nextIndex[i]设置为follower返回的index



# 在图2规则中为什么要保证leader提交的entry要满足log[N].term == currentTerm？

为什么我们不能提交任意的entry只要满足大多数都有这个entry了？

​	这种情况下提交的entry怎么会有可能被丢弃？

![提交日志覆盖](http://bos.nj.bpc.baidu.com/v1/agroup/08b43293b3d71a530c4b6e55c4ab3218d9d9b1f4)

- (a) S1 是领导者，部分的复制了索引位置 2 的日志条目
- (b) S1 崩溃了，然后 S5 在任期 3 里通过 S3、S4 和自己的选票赢得选举，然后从客户端接收了一条不一样的日志条目放在了索引2 处
- (c) S5 又崩溃了；S1 重新启动，选举成功，开始复制日志。在这时，来自任期 2 的那条日志已经被复制到了集群中的大多数机器上，但是还没有被提交
- (d)  S1 又崩溃了，S5 可以重新被选举成功（通过来自 S2，S3 和 S4 的选票），然后覆盖了他们在索引 2 处的日志。但是，在崩溃之前，如果 S1 在自己的任期里复制了日志条目到大多数机器上
- (e) 然后这个条目就会被提交（S5 就不可能选举成功）。 在这个时候，之前的所有日志就会被正常提交处理

该问题是因为：当一个新Leader当选时，由于所有成员的日志进度不同，很可能需要继续复制前面纪元的日志条目，因为即使为前面纪元的日志复制到多数服务器并且提交,如步骤C，但是在D中还是可能被覆盖，这就产生了不一致。解决的方法就是通过规则6：如果存在以个N满足 N>commitIndex,多数的matchIndex[i] >= N,并且 log[N].term == currentTerm:设置commitIndex = N，具体就是在c阶段，S1成为Leader，此时的纪元是4。S1一样向其他服务器发送日志2，当发送到多数服务器S1,S2,S3时，此时并不提交该日志，而是继续复制日志4,直到日志4到达多数服务器后，提交日志4，即leader只会提交当前纪元的日志。如果提交了4之后宕机，S5就不会被选举为新的 Leader，如果在提交4之前宕机，那么日志2,日志4还是可能被覆盖，但是由于没有提交，也就没有执行日志中的命令，即使被覆盖也无关系。

所以一个entry变为commited如果：

1）在他发送出去的term内达到了大多数的同意

2）如果后续的entry变为committed

这是因为：

"more up to date"的投票规则倾向于大的任期

leader会将日志同步给followers



# 持久化和性能

如果一个server挂了又重启，它必须要记住什么？

图2列出了"persistent state"：

- currentTerm
- votedFor
- log[]

一个Raft的server在重启后只有他们是完整的才能再次加入

因此必须要将状态存储到非易失性存储上如果改变了上面的变量

​	在发送rpc或者回复rpc的时候

non-volatile = disk, SSD

## 为什么要存储log[]？

如果重启的server之前回复了leader说我收到entry了，但是重启后，丢失了，那这条已经确认的日志可能就会在将来丢失了

## 为什么currentTerm/votedFor?

防止在一个term多次投票，导致多个leader的产生



## 那状态机的状态呢？需要存储嘛

不必要存储，因为存储了log，可以通过log重建出来

server重启后lastApplied会变为0，这个时候所有的log会重新应用到状态机

## 持久化存储一般是性能的瓶颈

一个硬盘操作消耗10ms，ssd消耗0.1ms

rpc消耗低于1ms

因此我们预期能有100到10,000操作/每秒

## 我们怎么提高性能？

在每个rpc和每个disk写中我们批量操作并发客户端的操作

使用更快的non-volatile存储

## 怎么获取更高的吞吐？

how to get even higher throughput?
  not by adding servers to a single Raft cluster!
​    more servers -> tolerate more simultaneous server failures
​    more servers *decrease* throughput: more leader work / operation
  to get more throughput, "shard" or split the data across
​    many separate Raft clusters, each with 3 or 5 servers
​    different clusters can work in parallel, since they don't interact
​    lab 4
  or use Raft for fault-tolerant master and something else
​    that replicates data more efficiently than Raft, e.g. GFS chunkservers

## Raft放弃了性能来获取简洁？

大多数复制系统都有类似的性能问题：

​	每次协商都涉及一次rpc交互和一次磁盘操作

一些Raft的设计选举可能影响性能：

​	Raft follower拒绝乱序的AppendEntries RPCs

​		而不是将请求存储起来，等待之前的AppendEntries到来后在处理

​		如果网络的原因导致乱序的包特别多，那可能上述的问题会特别重要

​	一个慢的leader可能会影响Raft：例如异地备份

## 在实际中什么对于性能特别重要呢？

你用它来做什么

批量处理rpc和硬盘操作

只读请求快速路由

高效的rpc库

Raft可能兼容大多数的这种技术

更多关于性能的论文：

  Zookeeper/ZAB; Paxos Made Live; Harp

# 主题：client behavior, duplicate operations, read operations

## 实验3的key/value客户端怎么工作？

client发送Put/Append/Get RPCs到servers

怎么找到leader？

如果leader挂了怎么办？



## client RPC loop

发送RPC给server

RPC将会返回：可能是错误（超时）或者收到响应

如果收到响应，server说是leader，返回

如果没有回复，或者server说不是leader，尝试发送给另一个server

client应该缓存最后一次知道的leader

## 问题：这会产生重复的客户端请求

client发送请求给leader S1，S1发送日志给力大多数，然后S1在给client回复的前crash了

客户端看到RPC错误，等待新的leader产生，然后重新发送请求

新的leader将请求又再次放入log

解决办法：我们需要一些**at-most-once**的机制

每个server应该记录已经执行过的客户端请求

如果一个请求再次出现在log中，不执行这个log，然后回复给client上次执行的结果

服务做了上面的事情，而不是Raft

lecture 2说明了怎么有效的检测重复请求



所有的副本都应该保存一份已执行请求的表，而不仅仅是leader，因为任意一个server都可能变为leader，客户端可能会重新发送请求

## 如果保存的返回结果不是最新的ok吗？

C1发送get(x)，servers执行后得到结果1，leader在回复前crash

C2发送put(x, 2)

C2重新发送get(x)给新的leader，如果从缓存的结果中得到回复，则是1

对回答正确结果的直觉是：应用从非副本的sever上能看到结果1吗？

是的！

C1发送了get(x)，执行非常快，但是由于网络的原因回复的非常慢，所以在C1收到结果1之前C2已经执行了put(x,2)

这种语义的正式名称是：**线性化（linearizable）**

每个操作在某个时间点执行，反应了该操作在执行时间点之前所有操作的结果

一个操作的时间是介于发送和收到之间

## 一个重要的问题，leader可以本地执行只读操作吗？

leader本地执行只读操作，而不会发送给所有的followers，并且等待commit？

很诱人的一个想法，因为r/o操作可能会占大多数，并且也不会改变状态

这为什么会是一个坏主意？

我们怎么做才能让这个想法实现呢？

只读的请求可以不写log就能执行，但是它有可能返回过期的数据，有如下场景：

- 老的leader挂掉了，但它自身还认为自己是leader，于是client发读请求到该server时，可能获得的是老数据

Raft通过如下方法避免上述问题：

- Leader必须有被提交条目的最新信息，这个指leader肯定是拥有最的log的机器，但是在前一个任期的log不知道是否已经提交了，是否已经应用到状态机了，因此无法保证数据是最新的，Raft采用,每个Leader在开始时提交一个空操作条目到他的日志中，此时通过提交本纪元的条目，自然而然就保证了之前纪元的条目被应用到状态机，从而获取最新的状态
- leader在执行只读请求时，需要确定自己是否还是leader，通过和大多数的server发送heartbeat消息，来确定自己是leader，然后再决定是否执行该请求



#  主题：日志合并和快照

问题：

- 日志会变的越来越大：远比状态机的状态大
- 会使用大量的内存
- 将会耗费大量时间：在重启后读取并且应用日志，同步日志给新加入的server

## 什么限制了服务器如何抛弃老旧的部分日志?

不能忘记un-committed的操作

在crash和restart需要回放

需要将最新的状态同步给其他server

## 解决方案：service周期性的创建持久的快照（snapshot）

复制整个状态机的状态通过一个特殊的日志条目

​	例如：k/v表，客户端状态

service将快照写到持久化存储

service通过一些条目告诉Raft这是快照

一个server能够在任何时候创建一个快照然后丢弃日志

## snapshot和logs的关系

快照（snapshot）只包含committed的日志

所以server只会丢弃committed之前的日志，任何未被确认为committed的日志都会留下

所以一个server的持久化的状态包括：

- service的一个快照，通过一个特定的日志条目
- Raft的持久化日志跟随着快照条目

以上两者等同于全量日志

## 当crash+restart的时候发生了什么？

service从磁盘读取快照

Raft从磁盘读取持久化日志

发送已经committed的条目但是不在快照中的

## 如果leader丢失了不在follower日志中的条目

nextIndex[i]将会回到leader日志开始地方

所以leader不能通过AppendEntries RPCs修复follower

那为什么leader不丢失只有所有server都有的日志？

## 什么是 InstallSnapshot RPC？

  term
  lastIncludedIndex
  lastIncludedTerm
  snapshot data



```

 what if leader discards a log entry that's not in follower i's log?
  nextIndex[i] will back up to start of leader's log
  so leader can't repair that follower with AppendEntries RPCs
  (Q: why not have leader discard only entries that *all* servers have?)

what's in an InstallSnapshot RPC? Figures 12, 13
  term
  lastIncludedIndex
  lastIncludedTerm
  snapshot data

what does a follower do w/ InstallSnapshot?
  reject if term is old (not the current leader)
  reject (ignore) if we already have last included index/term,
    or if we've already executed last included index
    it's an old/delayed RPC
  empty the log, replace with fake "prev" entry
  set lastApplied to lastIncludedIndex
  send snapshot in applyCh to service
  service *replaces* its lastApplied, k/v table, client dup table

The Question:
  Could a received InstallSnapshot RPC cause the state machine to go
  backwards in time? That is, could step 8 in Figure 13 cause the state
  machine to be reset so that it reflects fewer executed operations? If
  yes, explain how this could happen. If no, explain why it can't
  happen.

*** topic: configuration change

configuration change (Section 6)
  configuration = servers集合
  configuration = set of servers
  每隔一段时间你可能都想
  	变更为一个新的servers集合
  	增加/减少servers
  人进行配置的修改，Raft来操作
  我们希望Raft在配置更改期间也能正确执行
  every once in a while you might want to
    move to an new set of servers, or
    increase/decrease the number of servers
  human initiates configuration change, Raft manages it
  we'd like Raft to execute correctly across configuration changes

why doesn't a straightforward approach work?
  suppose each server has the list of servers in the current config
  change configuration by telling each server the new list
    using some mechanism outside of Raft
  problem: they will learn new configuration at different times
  example: want to replace S3 with S4
    S1: 1,2,3  1,2,4
    S2: 1,2,3  1,2,3
    S3: 1,2,3  1,2,3
    S4:        1,2,4
  OOPS! now *two* leaders could be elected!
    S2 and S3 could elect S2
    S1 and S4 could elect S1

Raft configuration change
Raft配置的更改
  idea: "joint consensus" stage that includes *both* old and new configuration
  leader of old group logs entry that switches to joint consensus
    Cold,new -- contains both configurations
  during joint consensus, leader gets AppendEntries majority in both old and new
  after Cold,new commits, leader sends out Cnew
  S1: 1,2,3  1,2,3+1,2,4
  S2: 1,2,3
  S3: 1,2,3
  S4:        1,2,3+1,2,4
  no leader will use Cnew until Cold,new commits in *both* old and new.
    so there's no time at which one leader could be using Cold
    and another could be using Cnew
  if crash but new leader didn't see Cold,new
    then old group will continue, no switch, but that's OK
  if crash and new leader did see Cold,new,
    it will complete the configuration change
```



