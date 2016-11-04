本文是读Efficient Optimistic Concurrency Control using LooselySynchronized Clocks的笔记

为什么阅读本文

	编程人员都喜欢事务，但是很难高效的实现

	Thor通过client cache获得速度，通过乐观并发控制获得了无锁消息

Thor概述

	[clients, client caches, servers A-M N-Z]

	数据在servers之间共享

	code在client上运行，而不是servers

	clients fetch/put objects (DB records) from servers

	clients对object进行本地cache，获得快速的访问

	

Client caching使得事务很tricky

	怎么处理读取到的数据是旧数据

	怎么得到并发控制（因为clients没有锁）？



A reminder:

	"serializable"：结果就像按顺序执行事务一样

	"conflict"：两个并发的事务使用record，至少一个做些操作



Thor uses optimistic concurrency control (OCC)

	只是从本地读写副本

		直到commit的时候才担心事务

	当事务想要提交：	

		发送读写信息到server进行"验证"

		验证成功则提交----if it can find equiv serial order

		if yes, 更新server的数据，发送cache失效消息给客户端

		if no,abort，放弃写

	"optimistic" b/c executes w/o worrying about serializability

  	"pessimistic" locking checks each access to force serializable execution



验证怎么做？

	查看已经提交/正在提交的事务读和写的内容

	决定是否能取得一个顺序的执行顺序

		和实际的并发执行一样的结果

	有许多的OCC验证算法

		我会列举出一些，从而会有一个简化的Thor

		FaRM则使用了另一个算法



验证规则 #1：

	一个验证服务器

	clients告诉验证服务器读和写的值

		每个事务看到的想要提交的数据

		"read set" and "write set"

	验证服务器必须要决定：

		如果我们让这些事务提交，结果是否是serializable

	scheme #1 打乱了事务，寻找一个串行顺序满足：

		每个读看到的写的最新值

		如果存在一个，则执行是serializable



验证例子1：

	初始化时：x=0 y=0 z=0

	T1: Rx0 Wx1

	T2: Rz0 Wz9

  	T3: Ry1 Rx1

  	T4: Rx0 Wy1	

	验证需要决定这个执行（读，写）相当于一些串行顺序嘛？

	yes：一个可行的顺序是：T4, T1, T3, T2  (really T2 can go anywhere)

		所以对四个事务都能说yes

	注意：这个规则比Thor宽容多了

		它允许事务能看到未提交的写

		因为四个事务都并行的执行，没有一个在validation的时候提交了，因此他们显然看到了彼此的写

	

OCC很简洁：b/c transactions不需要锁	！

	因此通过client的cache能够快速执行

	每个事务只有一个消息用于写的检测

		而不是每个record的使用都要检测锁

	

验证例子2：	有时候验证会失败

	期初：x=0 y=0

	T1: Rx0 Wx1

 	 T2: Rx0 Wy1

 	 T3: Ry0 Rx1

	值怎么限制潜在的顺序的？

	T1 -> T3 (via x)

    	T3 -> T2 (via y)

    	T2 -> T1 (via x)		

	上面有个环，因此不成立

	也许T3从cache中读到了一个旧的数据y=0

		或者T2从cache读到了x=0

	这种情况下，验证可以对T1，T2，T3中的任意一个说no

		另外两个就能通过了

client怎么处理abort？

	回滚程序（包括写到程序的内存中的数据）	

	从事务开头开始重新执行

	期望第二次不会冲突了

	当冲突罕见的时候，OCC是最好的

我们是否需要验证只读的事务？

	例子：初始时 x=0 y=0	

	T1: Wx1

    	T2: Rx1 Wy2

   	 T3: Ry2 Rx0

		可能是T3从cache中读到了x=0，还没收到失效消息

	没有满足条件的顺序

	所以我们对只读(r/o)的事务也需要验证

	其他的OCC能够避免只读的事务

		通过保持多个版本：但是Thor和我自己的规则不这么做



OCC优于锁嘛？

	是的，如果冲突少量的情况下

	no，如果有很多冲突

		OCC abort了，必须重新再执行，可能会多次

		locking等待，不需要重新执行

	例子：同时增加（simultaneous increment）

		locking:

     		 	T1: Rx0 Wx1

      			T2: -------Rx1  Wx2

		 OCC:

      			T1: Rx0 Wx1

      			T2: Rx0 Wx1

      			fast but wrong; must abort one	

We want distributed OCC validation

	拆分存储和验证负载到各个服务器上

	每个存储服务器只验证自己部分的xaction

	每个事务通过一个TC管理：可能每个Client都充当自己的TC

	TC要求每个server进行验证

		如果所有都回复ok，则告诉每个server进行commit

		这是一个两阶段提交提交（但只有写才lock）



我们可以对规则1进行分布式处理吗？

	例如：每个server说”yes“如果有顺序的编号

	考虑下面的场景：server1知道x，server2知道y

	例子2：

		T1: Rx0 Wx1

  		T2: Rx0 Wy1

    		T3: Ry0 Rx1

	S1只验证x的信息

		T1: Rx0 Wx1

    		T2: Rx0

    		T3: Rx1

		此时 T2 T1 T3 是ok的所以S1回答yes

	S2只验证y：

		T2: Wy1

    		T3: Ry0	

		此时T3 T2 是ok的，所以S2回答yes

	但是我们知道实际答案是"no"

所以此时本地的分布式规则#1不成立了

	验证必须需要选择	一致的 顺序



验证规则2：

	想法：client（或者TC）选择时间戳作（TS）为提交的动作

		时间来自于松散的同步时钟

	验证时检查读和写都是一致的，以TS为顺序

	解决分布式验证问题：

		timestamps告知了validators检查的顺序

		所以yes意味着顺序相同

Example 2 again, with timestamps:

  T1@100: Rx0 Wx1

  T2@110: Rx0 Wy1

  T3@105: Ry0 Rx1

 S1 validates just x information:

    T1@100: Rx0 Wx1

    T2@110: Rx0

    T3@105: Rx1

    timestamps say order must be T1, T3, T2

    validation fails! T2 should have seen x=1

  S2 validates just y information:

    T2@110: Wy1

    T3@105: Ry0

    timstamps say order must be T3, T2

    validates!

  S1 says no, S2 says yes, TC will abort



时间戳保证了不同的验证器检查的都是相同的顺序做检查



那我们什么时候会放弃timestamp的顺序呢？

	example:

    		T1@100: Rx0 Wx1

    		T2@50: Rx1 Wx2	

	T2会abort，因此TS说T2先来，T1应该看到x=2，但是T1可能已经提交了，因为先T1到来，然后T2才到的

	这种情况可能是客户端时钟相差比较大

	如果T1的时钟超前，T2的落后

	所以：要求TS order会造成不必要的abort

	b/c validation no longer searching for an order that works

	代替的是只需要检查TS order一致性读和写

	当时钟是非常同步的时候，问题不大

	当冲突很少的时候，问题不大

目前为止的问题:

	提交的消息中包含了值，可能数据很大

	可以通过每个对象一个版本号来检查读到的数据是否是之前xaction写的数据

	我们可以使用writing xaction's TS作为object的版本号

		也可以使用顺序号，例如在FaRM中

验证规则#3

	对每个DB record（包括cache中的record）都打上最后一个写action的TS

	验证请求将读的每个记录的record带过来

	检查每个读看到的version（按照ts序，是否是最近写的version）

	提交的时候，将写object's的版本设置为新的TS

规则3的例子：		

	所有的值开始的时候ts都是0

	T1@100: Rx@0 Wx

  	T2@110: Rx@0 Wy

  	T3@105: Ry@0 Rx@100		

	注意：

		读集合中object的是version不是values

		写集合中包含最新的值，但是不用来验证

	S1只验证x的信息：

		按ts排序

		T1@100: Rx@0   Wx

   		T3@105: Rx@100

   		T2@110: Rx@0

		问题是：是否每个读都看到了最新的值？

		T3是对的，但是T2不对

	S2 validates just y information:

    		again, sort by TS, check each read saw latest write:

    		T3@105: Ry@0

    		T2@110: Wy

		S2 ok

	这是Thor问题的一半回答：

	Q：如果transaction读到旧的值，怎么办？

	A：如果Thor使用version号，他们会告诉验证说T2读到了x的旧值	



我们考虑version而不是值，放弃了什么？

	可能version不同，但是值相同

		e.g.

 	   T1@100: Wx1

 	   T2@110: Wx2

 	   T3@120: Wx1

	   T4@130: Rx1@100

	versions说应该abort T4，应该读到T3写的版本

	但是其实T4读到了正确的值x=1

	虽然有这种情况，但是实践中也是ok的，好多occ系统都使用version

问题：每个记录一个version，可能会使用掉太多的存储空间

	Thor想要避免空间浪费

验证规则#4

	Thor的invalidation规则：记录上没有时间戳

	那验证怎么检查出 transaction读取到了旧的值呢？

	它读取到了旧的数据，因为之前xaction's的失效消息还没有到来

	Thor server 记录了每个client可能还没有看到的失效消息 （invalidation msgs）

	"invalid set" -- one per client

	删除失效的集合项，当客户单 ack invalidation msg

	server放弃committing的xaction，如果在client的失效集合中读到record

	客户端放弃running xaction如果在失效消息中的record

	

使用失效集合的列子：

	[timeline]

  	Client C2:

    		T2 ... Rx ... client/TC about to send out prepare msgs

  	Server:

    		T1 wrote x, validated, committed

    		x in C2's invalid set on S

    		server has sent invalidation message to C2

    		server is waiting for ack from C2	

3个cases：

	inval消息在C2发送prepare消息前到达C2

		C2 aborts T2

	inval在C2发送完prepare后，等待 yes/no的回答时

		C2 aborts T2

	inval丢失或者延迟了(so C2 doesn't know to abort T2)

		S没有收到C2的ack

		C2还在S上的x的inval set

		S会对T2的prepare消息回复说no

	so: Thor's validation detects stale reads w/o timestamp on each record

	  this is the full answer to The Question	

    性能
    Look at Figure 5
      AOCC is Thor
      comparing to ACBL: client talks to srvr to get write-locks,
       and to commit non-r/o xactions, but can cache read locks along with data
      why does Thor (AOCC) have higher throughput?
        fewer msgs: commit only, no lock msgs
      why does Thor throughput go up for a while w/ more clients?
        apparently a single client can't keep all resources busy
        maybe due to network RTT?
        maybe due to client processing time? or think time?
        more clients -> more parallel xactions -> more completed
      why does Thor throughput level off?
        maybe 15 clients is enough to saturate server disk or CPU
        abt 100 xactions/second, about right for writing disk
      why does Thor throughput *drop* with many clients?
        more clients means more concurrent xactions at any given time
        more concurrency means more chance of conflict
        for OCC, more conflict means more aborts, so more wasted CPU
      
    Conclusions
      caching reduces client/server data fetches, so faster
      distributed OCC avoids client/server lock traffic, again for speed
      loose time sync helps servers agree on equivalent order for validation
      distributed OCC still a hot area 20 years later!

结论

	caching减少了client/servr之间的数据fetch，所以快

	分布式OCC避免了clien/server的锁争用

	松散的时间同步帮助servers对顺序达成一致，达到检测的目的

	分布式OCC在20年后仍然是一个热门领域


