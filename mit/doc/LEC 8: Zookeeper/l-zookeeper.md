Ordering guarantees
    所有写操作有序（totally ordered）
    每个client的请求FIFO
        同一个client，后面的读请求能看到之前写的最新数据
        一个读请求可能看到的数据不是最新的
        
Zookeeper simplifies building applications but is not an end-to-end solution
    zookeeper只是提供了基础的同步元语，但是具体的协议还需要具体应用具体设计
 
Implementation overview
    两层结构，上层是kv即数据层，而下面是一致性层
    Start()将操作在ZAB层持久化，然后按顺序传递到上层kv
    kv将操作安顺序执行，得到最新的状态
    two layers:
        zookeeper services  (K/V service)
        ZAB layer (Raft layer)
        Start() to insert ops in bottom layer
        Some time later ops pop out of bottom layer on each replica
            These ops are committed in the order they pop out
            on apply channel in lab 3
            the abdeliver() upcall in ZAB

挑战：客户端请求重复
    场景：客户端发送请求给primary，失败了，重新发送请求给新的primary
    解决方案：
        lab3使用table来检测重复
     限制：每次只能有一个请求，不能进行pipeline
     Zookeeper：
        有些操作是幂等周期（Some ops are idempotent period）
        有些操作是很容易实现幂等的：test-version-and-then-do-op
        有些操作是由客户端来去重的
            Consider the lock example.
                   Create a file name that includes its session ID
                     "app/lock/request-sessionID-seqno"
                     zookeeper informs client when switch to new primary
            	 client runs getChildren()
            	   if new requests is there, all set
            	   if not, re-issue create

挑战：读请求返回的数据不是最新的
    当读的时候，可能之前的写请求只是在leader执行了，follower还没有同步到
    另一种情况是由于网络分区，读到的是old leader的数据
zookeeper的解决方案：并不承诺是最新的数据
    读请求会返回当前读可见的最后一个**xid**
    所以当新的primary接受请求的时候，能够至少同步到xid再处理读请求
    只有sync-read() 才会保证数据的是最新的，sync-read() 会通过Zab协议
同步的优化：避免ZAB在一般的情况下
  leader puts sync in queue between it and replica
    if ops ahead of in the queue commit, then leader must be leader
    otherwise, issue null transaction
      allows new leader to find out what last committed txn is
    in same spirit read optimization in Raft paper
      see last par section 8 of raft paper
    
    