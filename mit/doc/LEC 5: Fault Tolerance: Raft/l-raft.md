# 6.824 2016 Lecture 5: Raft (1)

## 概要：使用状态机(SRM)来实现容错服务

[clients, replica servers]

每个副本以相同的顺序执行相同的操作

结果：每个副本随着执行都保持一致

如果其中一个挂了，任意一个副本都能够取代，例如：客户端能连接任意一个副本

GFS和VMware FT都通过SRM来实现容错

## 典型的"state machine"

可能是一个应用或者服务，他们都需要被复制

内部状态，输入指令序列，输出

​	序列意味着没有并行

​	必须是确定性的

​	只能通过(输入和输出)和状态机通信

从现在开始我们会讨论一些比较具体的服务

例子：配置服务器，如MapReduce和GFS master

例子：key/value存储服务，put()/get()



## 一个关键问题：怎么避免出现多主

假设client和副本A通信，不和副本B，那client能只和A通信嘛？

如果B真的crashed，我们应该要能继续处理，否则系统就不是容错得了

如果B恢复了，但是由于网络原因，我们不能和B进行通信，此时我们的系统不应该继续工作，因为B此时可能是正常工作的，正服务着其他的客户端，这就是所谓的**"network partition"**问题

通常情况下，我们一般无法区分故障和分区



## 通过单主来避免集群分裂（split brain）

master来决定A和B中哪个是主要的（primary），因为只有一个master，不会存在意见不一致

客户端和master通信

但是如果master故障了呢？有单点的问题

## 理想的状态机模型

没有单点问题，即使一个server故障也能继续工作

可以处理分区问题



## 处理分区问题的方法：大多数投票

有2f+1个服务器

需要得到大多数（f+1）票才能继续（成为主primary），因此可以在即使故障f个server的情况下也能工作，没有单点故障

为什么没有集群分裂的问题？因为最多只有一个server能获得大多数投票

注意：大多数意味着2f+1的大多数，而不是存活着的server的

大多数需要考虑的关键点事：任意两个都需要交互。交互的server只会其中一个投票，而且交互中还会携带其他信息

## 两个主要的复制协议是在1990年左右发明的

 Paxos 和 View-Stamped协议

在过去10年终这项技术在实践中大量运用

Raft是一个特别优雅的想法

## MapReduce, GFS, 和 VMware FT都从SRM中受益

MapReduce master没有备份

GFS master有备份，但是没有自动切换到备的策略

VMware FT共享硬盘，test-and-set显然不是备份

## Raft的状态机复制

大于等于3个servers

Raft选出一个为leader

client发送rpc到leader的k/v层

leader发送每个客户端的命令给所有的备份

​	每个follower新增日志

​	目标是所有servers都有相同的日志

​	将并发的客户端请求Put(a,1) Put(a,2)排序得到相同的顺序

日志条目被提交如果大多数都存储了该条目：大多数意味着即使少量的servers故障了也能继续服务

servers执行日志条目一旦leader说条目已经提交：k/v 应用put到DB，或者得到Get的结果

leader回复给client结果

## 为什么需要日志？

service保存着状态机的状态，除此之外为什么还不够？

如果一个follower丢失了leader的部分命令呢？

​	怎么能有效的将同步到最新？

​	答案：leader重新发送丢失的命令给他

log是命令的序列化存储

​	starting state + log = final state

log同样提供了一个方便的编号方案：能够给操作排序

同时log也是一个命令暂存的地方：直到我们确认命令已经提交了

## Raft的日志总会有确定的副本嘛？

不会：一些副本可能会落后

不会：可能会有不同的日志

但是好消息是：

​	如果一个server已经在指定的序列号上执行了一个命令，那不会有其他server在同样的序列号上执行不同的命令



## lab 2 Raft interface

rf.Start(command) (index, term, isleader)

​	提交一个新的日志条目

​	马上返回，稍后可能成功或者失败

​	如果server在提交命令前失去了领导权会失败

​	返回的index表示如果日志提交成功，将会返回的序号

ApplyMsg，传递下标和命令

​	Raft在channel上产生一个消息当service应该执行一个新的命令的时候，这同时也告知leader可以给客户端返回响应了

注意：leader不应该等待每个follower对AppendEntries的响应，如果等待leader会被阻塞，因此开启新的goroutine来发送消息，带来的问题是：rpc会乱序，后发的AppendEntries可能会先到



## Raft设计主要包含两部分

- 选举一个新的leader
- 失败后保证日志相同

## Raft对leader进行编号

新的leader -> 新的任期（term）

一个任期最多只有一个leader，可能会没有

编号可以帮助servers跟随最新的leader，而不是过时的

## Raft什么时候开始一个leader的选举

当servers没有听到leader的通知，于是增加本地的任期，变为候选人，开始选举



## 怎么保证在一个任期至多只有一个leader

leader必须要获得多数票

每个server在每个任期只能投出一票：投给第一个请求的候选人

对于一个给定的任期，至多只有一个server能获取到大多数票

​	即使网络分区也最多一个

​	即使少量的server故障了，选举也能胜出

## 一个server怎么知道选举胜出了？

胜利者得到了大多投票

其他人会收到leader的AppendEntries心跳

## 什么时候选举会没有结果？

当出现split vote，即没有一个server得到了大多数投票

少于大多数的server可以收到投票请求

## 选举失败后会发生什么？

重新开始选举

## raft怎么减少由于split vote造成的选举失败

每个server开始选举前延迟一个随机时间

## 怎么选择随机延迟？

太短：第二个选举在第一个没结束的时候就开始了

太长：当leader故障后，系统等待选举太长

一个粗略的指导：

​	假设完成一个选举需要10ms并且有5个servers，我们希望延迟以20ms相隔，那随机延迟从0到100ms



## Raft的选举遵循一个通用的原则：分离安全和进程

硬排除机制：排除大于1个leader的情况，避免集群分裂，但是可能会没有leader

软机制保证发展：开始一个新的选举总是安全的

软机制尽量避免不必要的选举

​	来自leader的心跳（告诉follower不要开始新的选举）

​	超时周期（不用太快开始选举）

​	随机延迟（给leader足够的时间来选举出来）

## 如果旧的leader没有意识到新的leader产生会怎么样？

也许旧的leader没有看到选举消息

新的leader意味着大多数servers已经新增了任期（currentTerm）

​	所以旧的leader的操作不会得到大多数人支持

​	所以旧的leader不会提交或执行新的log

​	因此即使发生了分区也不会出现split brain

​	但是一小部分server可能会收到旧leader的AppendEntries

​		所以日志在旧的任期的尾部会出现不一致



接着讨论系统故障后，日志怎么同步

## 我们需要保证什么？

也许：每个sever以相同的顺序执行相同的命令

因此：如果任意一个server执行了，那么不会执行其他的命令了

只要leader存在，很容易防止日志的不一致

leader强制每个follower都需要和自己保持一致
