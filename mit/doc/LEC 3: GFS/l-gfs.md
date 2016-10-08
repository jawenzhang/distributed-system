## 为什么读这篇论文

map/reduce的文件系统

处理存储故障的案例

​	为了系统的简单和性能牺牲一致性

​	后续设计的一个动机

高性能：高效的并行IO

优秀的系统设计论文：包含从应用到网络的所有细节

包含了6.824所有主题：性能，容错，一致性

## 什么是一致性

正确性条件（A correctness condition）

当数据需要复制或者被应用并行访问的时候特别重要

​	如果应用先执行了一个写操作，这个时候马上进行一个读，会读取到之前写入的数据吗？如果这个时候另一个应用来读，会读到最新的数据吗？

一致性有2种类型：

1. 弱一致，读操作可能会返回旧的数据，不是最新写入的数据
2. 强一致：读操作总是返回最新写入的数据

强一致适用于应用的写，但是性能不好

需要的一致性条件（也称为一致性模型）



## 理想的一致性模型

复制的文件访问起来像没有复制一样

​	就像在同一机器上的多个客户端访问同一硬盘上的文件

如果一个应用先写，接着读的应用能读到新数据

如果两个应用并发的写到同一个文件

​	在文件系统中，这种行为是未定义的——— 文件内容通常是写乱的

如果两个应用并发的写到同一个文件夹

​	一个先写，另一个再写



## 不一致性的原因

- 并发
- 机器故障
- 网络分区



## GFS中的例子

主和备份B之间网络不通

客户端新增1

主发送1给自己和备份A

报告错误给客户端

同时客户端2可能会从B读取数据，此时读到旧数据



## 在分布式系统中为什么保证一致性特别难呢？

协议会非常复杂 — — — 在下一周中讲解

​	保证系统的正确非常难

协议要求客户端和服务端之间进行通信

​	可能会损失性能



## GFS放弃了理想方案来获取更好的性能和简单的设计

应用开发者使用起来会非常困难

​	应用需要考虑在非理想系统中的一些问题

​	譬如：读取到旧的数据，重复的添加数据

​	但是数据不是你的银行账户，因此即使重复或者旧数据也ok

GFS论文是在下面几个点中的一种权衡：

- 一致性
- 容错
- 性能
- 系统的简易性

## GFS的目标

创建一个共享的文件系统

管理成百上千的物理机器

存储海量的数据



## GFS存储的是什么

作者没有明确的说明

在2003年时候的猜测是

​	索引 & 数据库

​	网络上的html文件

​	网络上的图片



## 设计概览

目录，文件，名字，open/read/write

​	但是是非POSIX

100个linux的chunk服务器

​	chunk大小64M

​	每个chunk复制到3个服务器

​	为什么是3份？

​	除了数据的可用性，3份数据还给了我们什么？热点文件的负载均衡和亲和力

​	那为什么不在RAID的存储一份拷贝？RAID不是日用品，希望整个机器都是容错，而不仅仅是存储设备

​	为什么chunk很大？

GFS的master服务器知道目录的层级

​	知道目录下面存储了哪些文件

​	知道文件的每个chunk存储在哪个chunk sever

​	master将这些信息都存在内存中

​		每个chunk存储64byte的metadata

​	master用私有的可恢复的数据库来存储metadata

​		master可以在断电后快速重启

​	备份master相比较master有一点状态上的延迟

​		可以在发生故障的时候提升为主



## 基本操作

客户端端：

​	发送文件名和偏移量到master

​	master将包含数据的chunk servers返回给客户端

​		客户端能够将这些信息在本地缓存一会儿

​	client请求最近的chunk sever

客户端写：

​	请求master什么地方去写

​	如果超过64M，master选择一组新的chunk servers

​	其中一个chunk server是主primary

​	主选择更新的顺序并且将数据备份到其他两个servers上



## 两种不同的容错计划

- master
- chunk servers



## master容错方案

单主

​	客户端总是和master发生交互

​	master负责给所有操作排序

存储着有限的持久化信息

​	命名空间（文件夹）

​	文件到chunk的映射

对于上面两个的变更都存储到log中

​	log做了多备份

​	客户端任何修改状态的操作都是在log中记录后才返回的

​	log在好多系统中都扮演着中心角色

log大小的限制

​	作为master状态的一个checkpoint

​	移除在checkpoint之前的所有操作

​	checkpoint复制到备份机上

恢复

​	从checkpoint重放log

​	chunk的信息通过询问每个chunk servers重新恢复出来

master是系统中的一个单点

​	因为master的状态非常小，因此恢复起来很快，因此不可用的时间非常短

​	备份master：落后于master，通过复制的log重放来恢复

​	如果master不能恢复，重新启动一个master，必须要小心启动两个master起来

​		

## chunk容错方案

master保证chunk在其中的一个副本中，那个副本就是primary chunk server

primary决定了操作的顺序

客户端将数据推送给副本

​	副本组成了一个链路

​	链路代表了网络的拓扑

​	允许快速复制

客户端发送写请求到primary

​	primary分配sequence

​	primary先将变化写到本地

​	primary将请求发给副本

​	primary当收到所有的副本的确认后，给客户端返回响应

如果一个副本没有响应，客户端进行重试

  Master replicates chunks if number replicas drop below some number
  Master rebalances replicas

​	

## chunks的一致性

一些chunks可能会是旧数据，因为错过了某些变化

通过chunk的version号来检测到旧数据

​	在处理租赁前，增加chunk的version号，将其发送到primary和备份chunk servers，master和chunk servers持久化存储version

同时发送version号给客户端

version号可以使得master和client探测到旧的数据



## 并发的写/追加(append)

客户端会并发的写到文件的同一个区域，写的结果是不做保证的

许多客户端并发的append（譬如日志文件），GFS支持原子，至少一次的语义

primary选择新增记录的偏移，发送给所有的副本，如果其中一个失败，primary将操作告知客户端

客户端重试，如果重试成功：其中有些记录会追加两次；也有可能文件会有空洞， when GFS pads to chunk boundary, if an append would across chunk boundary



## 一致性模型

对于目录操作是强一致模型

​	master对于metadata的更改都是原子操作

​	目录的操作都遵循理想模型

​	但是，当master下线的时候，只有备机，只会允许读操作，此时可能会返回旧数据

对于chunk的操作都是弱一致

​	一个失败的变更会导致chunks不一致

​	primary更新了chunk，但是复制给副本的时候失败了，此时副本就是老数据了

