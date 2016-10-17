# 主/备复制

主要是用于解决可用性问题，当一个不够的时候，再加一个备份，但是带来的问题是：主备之间的数据一致性问题，而且需要强一致



# 容错（FT）

即使发生故障，我们也希望服务能够继续

可用性：系统即使发生某些错误也能可用

强一致：对于客户端来说就像访问单一服务器

​	比GFS提供的文件的一致性还强

实现起来非常困难，但是非常的有用



# 需要一个错误模型，认识我们将会处理什么

独立的故障停止计算机错误（Independent fail-stop computer failure）

​	在VM-FT中更进一步，假设同一时间只有一个错误

全站停电（最终重启）

网络分区

没有bug，没有恶意错误



# 核心观点：复制

两个server（或者更多）

每个副本都持有要提供服务所需的状态

如果一个挂了，另一个副本能快速接手



# 例子：MapReduce master的容错

实验1中的worker设计的就是容错的，但是master没有，是单点

我们是否可以有两个master，预防其中一个挂了

保存的状态有

- worker列表
- job的完成状态
- worker的空闲状态
- TCP连接状态
- 程序计数器

# 主要的问题

1. 哪些状态需要被复制？
2. 副本怎么获取状态？
3. 什么时候转换到副本？
4. 转换期间异常是否可以被感知到？
5. 怎么恢复？

# 主要有两种方案

1. 状态传输（State Transfer）

   主（master）执行操作，然后将状态发送给备（backup）

2. 复制状态机（Replicated state machine）

   所有的状态机执行相同的操作

   如果起始状态相同，操作相同，顺序相同，没有不确定操作，则结果相同

# 状态传输相对简单

但是状态可能更大，传输会慢

VM-FT使用复制状态机

# 复制状态机更高效

基于操作远比状态数据小

但是更复杂：需要考虑顺序，多核，确定性

# 什么是复制状态机

K/V操作？

x86指令？

影响性能和实现的复杂度

​	主备之间需要多少交互，更高层级的抽象会带来更少的信息交互

​	在x86上处理非确定行为很难，但是在put/get层级会相对容易



# 论文导读

```
The design of a Practical System for Fault-Tolerant Virtual Machines
Scales, Nelson, and Venkitachalam, SIGOPS OSR Vol 44, No 4, Dec 2010
```

非常有目标的系统：

​	整个系统都是可复制的

​	对应用和客户端透明

​	如果可以运转起来将会是非常不可思议的

​	故障模型：

​		独立的硬件故障	

​		断电

​	限于单处理的器的虚拟机

概述

​	两个机器，一主一备

​	两个网络：clients-to-servers, logging channel

​	共享存储

​	备机紧跟主

​		主发送所有的输入过来

​		备的输出丢弃

​	主备之间的心跳

​		如果主挂了，快速启动备

什么是输入

​	clock interrupts  

​	network interrupt  

​	disk interrupts	



挑战

1. 让系统看起来像只有一个server

   What will outside world see if primary fails and replica takes over?

   如果primary在回复客户端之前或之后故障，客户端请求是否会丢失或者执行两次？

   primary什么时候回复给客户端？

2. 怎么避免出现两个primary？
   如果`logging channel`故障？

   会出现primary和backup都成为primary？



3. 怎么保证主备同步？

   什么操作必须传递给backup？时钟中断？

   怎么处理非确定性操作（non-determinism）？例如：中断必须和指令一起在backup上执行


挑战3解决方案：deterministic replay

​	目标：使得x86平台deterministic

​		想法：使用hypervisor来使得虚拟的x86平台deterministic

​			两阶段：日志和回放

​	将所有的硬件事件记录到日志中

​		clock interrupts, network interrupts, i/o interrupts 等

​		对于non-deterministic指令，记录额外的信息

​			例如：记录time stamp register的值，回放的时候直接返回time stamp register的值

​	回放（relay）：以同样的顺序同样的命令执行

​		同时也在相同的命令点回放中断

​	限制：只针对单核x86，不针对多核

​		因为记录多核的命令交错执行，代价太大了

 deterministic replay应用到VM-FT

​	primary上的管理程序

​		发送log到备backup

​	backup上的管理程序重放日志

​		我们需要x86的虚拟机在下一个事件执行的指令上停止

​		我们需要知道下一个事件是什么

​		-> backup落后于一个事件



例子：

​	primary收到网络中断

​		监控程序（hypervisor）将中断和必要的数据发送给备机

​		监控程序（hypervisor）将中断发送给操作系统内核

​		操作系统内核执行

​		内核发送包给服务器

​		内核发送响应到网卡

​		监控程序获得控制权，将响应发送出去

​	备份收到日志

​		backup递送中断

​		backup发送中断给内核

​		。。。

​		不同之处在于备机上的监控程序会忽略输出

挑战1解决方案： FT protocol

​	primary直到backup确认后才输出

​		对于输出也会记录日志

​		primary在输出前记录日志，等到备份确认收到后才输出

​	性能优化

​		primary在输出操作停止的时候继续执行

​		对于输出操作buffer，然后等到备份确认后再输出

问题：为什么发送输出事件给backup，并且等到backup确认后才输出？

​	假设：我们只记录输入，不记录输出

​	primary：

​		处理网络输入

​		输出

​		发生故障

​	backup不能正确的重新产生该行为

​		最后一个日志是处理网络输入，将其发送给内核

​		backup启动（goes "live"），变为primary

​		硬件中断（例如：时钟），将其发送给内核

​		网络输入处理结果放入输出缓冲中

​			primary产生输出实在中断前还是后？

​			backup不知道，因为只有输入的log，没有中断的log也没有输出log

​		但是中断非常重要，可能会影响输出

​	通过将输出事件发送给backup，backup能正确的处理事件顺序

问题：主和备能产生相同的输出吗？

​	对于输出产生两次是没有问题的，因为primary发送输出log，但是在产生输出的时候故障了，此时backup无从得知是否发送输出到客户端了，backup重新产生输出，这个是没问题的

​	对于网络输出，客户端需要能处理重复包，对于写到disk，写同样的数据也没有问题

问题：主在收到网络请求后，发送log到backup前故障？

​	没有问题，客户端有重试机制	

挑战2解决方案： shared disk

​	只考虑不可靠网络的问题		