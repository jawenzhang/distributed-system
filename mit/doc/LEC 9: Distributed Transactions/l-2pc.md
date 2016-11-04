Topics

​	分布式事务=分布式提交+并发控制

​	两阶段提交

分布式提交（Distributed commit）：

​	一组计算机协议完成某些任务；例如：银行转账

​	每个计算机都有不同的角色，src和dst银行账户

​	希望保证原子性：要么都执行，要么都不执行

​	挑战：失败（failures），性能

Example：

​	日历系统，每个用户都有自己的日历

​	想要安排会议，需要和多个参与者

​	[一个client，2个servers]

​	sched(u1, u2, t):    

​		begin_transaction      

​			ok1 = reserve(u1, t)      

​			ok2 = reserve(u2, t)      

​			if ok1 and ok2:      

​				commit      

​			else        

​				abort    

​		end_transaction

​	reserve()调用时rpc调用，发送给两个日历服务器

​	我们想要保持原子性：同时保留或者都不保留

​	如果第一个reserve()返回成功：

​		第二个返回失败(time not available)

​		第二个没有返回(lost RPC msg, u2's server crashes)

​	我们需要一个“分布式提交协议”

想法：试提交，later commit or undo (abort)

​	reserve_handler(u, t):   

​		 if u[t] is free:      

​			temp_u[t] = taken -- A TEMPORARY VERSION      		

​			return true    

​		else:      

​			return false  

​	commit_handler():    

​		copy temp_u[t] to real u[t]  

​	abort_handler():    

​		discard temp_u[t]	

想法：只有一个人来决策是否提交

​	为了防止任何可能产生的不一致

​	我们会有一个Transaction Coordinator (TC)

​	[client, TC, A, B]

​	client 发送RPCs给A和B

​	在end_transaction的时候，client发送 "go"给TC

​	TC/A/B执行分布式提交协议

​	TC回复	"commit" or "abort" 给客户端			

我们希望分布式提交协议满足两个特性：

​	正确性：

​		如果任意一个commit，则不会有失败（abort）

​		如果人一个abort，则不会有提交成功的

​	性能：（因为不做任何事情就是正确的。。。）

​		如果没有失败（failures），那A和B可以提交，那就提交

​		如果失败，进入conclusion ASAP

我们会开发一种叫做“two-phase commit”的协议

​	被分布式数据库用来作multi-server事物

两阶段提交，没有failures的情况：

​	[client, TC, A, B]

​	client发送reserve()给A，B

​	client发送go给TC

​	TC发送“prepare”消息给A和B

​	A和B回复消息：我们是否准备好提交了

​		回复“yes”，如果没有crashed, timed out, &c

​	如果A和B都回复说"yes"，TC发送"commit"消息

​	如果任意一个说“no”，TC发送“abort”消息

​	A/B收到commit消息后，进行提交

为什么上述协议到目前为止是正确的？

​	A和B不能单方面提交，只有当两者都同意才行

​	至关重要的是：在回复prepare后，没有任何的改变产生，此时即使failure也没有问题

那出现了failures怎么办？

​	Network broken/lossy/slow 

​	Server crashes  

​	What is our goal w.r.t. failure?

​		恢复后继续执行correct操作

​	Single symptom：等待消息的时候超时了

hosts在哪等待消息？

​	1）TC等待yes/no

​	2）A和B等待prepare和commit/abort消息

Termination protocol 总结：

​	TC t/o for yes/no -> abort  

​	B t/o for prepare, -> abort  

​	B t/o for commit/abort, B voted no -> abort  

​	B t/o for commit/abort, B voted yes -> block

TC等待A/B回复yes/no的时候超时了

​	TC此时没有发送任何的commit消息

​	因此可以安全的发送“abort”消息

A/B等待TC的prepare消息时候超时了

​	此时还没有响应prepare消息，所有TC无法决定是否commit

​	所以A/B可以单方面的abort

​	将来收到的“prepare”的时候，回复no

A/B等待TC发送来的commit/abort的时候超时

​	我们以B为例（A同样的流程）

​	如果B说no，它可以单方面的abort

​	但是如果B回复了yes呢？

​	B可以单方面的abort嘛？

​	NO！TC也许已经同时从A/B得到yes了

​	已经发送commit给了A，但是在发送commit给B之前crash了

​	此时如果Babort了，那会造成A/B不一致

​	B不能单方面的commit，即使：

​		A可能回复的是no

如果B回复的yes，那它必须阻塞，等待TC的决定

如果B crashes并且重启了？

​	如果B在crash之前回复了yes，那B必须记住

​	重启后不能改为no

​	因为TC可能已经看到了之前的yes，并且告知A要commit了

因此参与者必须将状态持久化到硬盘：

​	B必须在回复yes之前记录状态，包括修改的数据

​	如果B重启了，硬盘上状态是yes，但是没有commit，此时需要主动询问TC

​	如果TC回复commit，则B将修改的数据变更为真实的数据

如果TC crash并且重启了呢？

​	如果TC在发送"commit" or "abort"之前crash了，TC必须记住！

​		如果重启后A/B询问，需要回复正确的决策

​		因此TC必须要在发送commit消息之前先写硬盘

​	TC不能对做出的决定进行更改，因为A/B/Client可能已经acted

这个协议称为两阶段提交：

​	所有的hosts做出的决定一致

​	除非所有人说yes，不然不会提交

​	TC failure会阻塞servers，直到TC恢复

并发的事物会怎么样?

​	我们经常同时希望有并发控制和原子提交		

​	x和y是银行账户

​	x和y刚开始都有$10

​	T1正在做转账操作,从x到y转$1

​	T1:    

​		add(x, 1)  -- server A    

​		add(y, -1) -- server B  

​	T2:   

​		tmp1 = get(x)    

​		tmp2 = get(y)    

​		print tmp1, tmp2	

问题是：

​	如果T2在两个add()之间执行

​	T2可能会打印出11，10

​	money凭空创造出来了

​	T2应该打印出10，10或者9，11

传统的方法是提供“serializability”

​	results should be as if transactions ran one at a time in some order

​	以某个顺序按个执行事务

​	as if T1, then T2; or T2, then T1    	

​		the results for the two differ; either is OK

你可以通过下面的方式测试一个特定的执行是否是“serializability”

​	找到顺序执行的编号，会产生相同的结果

​	顺序执行T1，T2

​	先执行T1，后执行T2产生9,11

​	先执行T2，后执行T1产生10，10

​	对于(11,10)没有这种顺序，但是对于10,10 and 9,11则有

为什么对于程序员来说serializability是好的？

​	允许应用能够忽略并发的可能

​	只需要写transaction让系统从一个状态变为另一个

​	内部来说：事务临时会打破不变量

​		但是serializability保证了没人会注意到

为什么serializability对于性能也是ok的？

​	没有冲突的事物可以并发执行

两阶段的locking是实现serializability的一种方式

​	每个database record都有一个锁

​	锁在存储记录的servers上存储着

​	每次使用记录都自动的等待、获取record的锁

​		因此add()操作隐性的获取了锁

​	锁会在commit或abort后释放

为什么在commit/abort后释放锁？

​	如果不这样，会违反serializability

What are locks really doing?

​	当事务冲突的时候，locks将一个延迟，强制顺序执行

​	当事务不冲突的时候，locks允许并行执行

locking和两阶段提交怎么交互的？

​	servers必须获取并且记录锁，当执行client请求的时候

​		所以client->server的rpc有两个效果：获取锁，使用数据

​	server必须保留prepared transaction的锁在 crash+restart的时候

​		因此，当收到prepare消息的时候，必须要记录锁的状态到磁盘上

​	但是restart后，对于一个非prepared transaction释放锁是ok的

​		    	probably implicitly by not even writing them to disk.    

​			as long as server then replies "no" to the TC's prepare.

2PC perspective		

​	场景是：共享DBs，当一个transaction在多个分片上使用数据

​	但是有一个不好的名声：

​		慢：因为需要多阶段、多消息的交换

​		锁在prepare/commit期间一直持有，阻塞了其他xactions

​		TC的crash，可能会导致无限等待，并且锁会一直占有

​	因此一般只在单个非常小的领域使用

Raft and two-phase commit solve different problems!

​	使用Raft来获取高可用，手段是复制

​		例如：即使一些服务器crash了，也能提供服务

​		所有的servers都做同一件事

​	使用2PC当每个参与者做不同的事

​	2PC对于可用性没有帮助

​		因为所有的服务器都必须完成工作

​	Raft并不保证所有的服务器都做了某事

​		因此只需要大多数机器存活就好

What if you want high availability *and* distributed commit?

​	每个server都应该是一个Raft-replicated service

​	TC应该是Raft-replicated

​	在replicated services之间使用2PC协议

​	此时就能接受failures，事务仍然能进行下去

​	我们将会在实验4中实现这个方案