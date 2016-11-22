## What is a memory model?

定义了不同线程交互的时候怎么进行读、写操作

契约：

​	给予compiler/runtime/hardware可优化的自由

​	给予programmer可依赖的保证

有许多的memory models

​	With different optimization/convenience trade-offs

​	一般称为一致性模型（Often called a consistency model.）



```
These terms refer to formal definitions of the results that different
  observers can see from concurrent operations on shared data.
```

当不同的observers观察并发操作共享数据时看到的结果

- Linearizability and sequential consistency（前者比后者更严格）

> Individual  reads  and  writes  (or   load  and  store  instructions).  Both linearizability  and sequential  consistency  require that  load results  be consistent with  all the  operations being  executed one at  a time  in some order. They  differ in that  linearizability requires  that if I  observe my store to complete, and I tell you so  on the telephone, and then you start a load of the same data, you will see my written value. Sequential consistency doesn't guarantee that: loads can  become visible to other CPUs considerably after  they appear  to  the  issuer to  complete.  Programmers would  prefer linearizability, but  real-world CPUs  mostly provide semantics  even weaker than  sequential consistency  (e.g. Total  Store Order).  Linearizability is seen more often  in the world of  storage systems; Lab 3,  for example, will have linearizable puts/gets.

- Strict serializability and serializability(用于数据库事务)

> the corresponding notions for transactions, where a transaction can involve reads and writes of multiple records. Both require that the transactions yield results as if they executed one at a time in some order. Strict serializability additionally requires that if I see my transaction complete,and then you start your transaction, that your transaction sees my transaction's writes. Programmers often think of databases as providing strict serializability, but in fact they usually provide somewhat weaker semantics; MySQL/InnoDB, for example, by default provides "Repeatable Reads", which can allow a transaction that scans a table twice to observe records created by a concurrent transaction.    

The Thor paper uses "external consistency" to mean the same thing as "strict serializability". 



What specific problems with previous DSM are they trying to fix?需要解决DSM的什么问题？
**false sharing**: two machines r/w different vars on same page两个机器同时读/写不同的变量，但是在同一页
​	M1 writes x, M2 writes y
​	M1 writes x, M2 just reads y
​	Q: what does the general approach do in this situation?
write amplification: a one byte write turns into a whole-page transfer

问题是：写放大，即使一个byte的写都需要将整个page传递

First Goal: eliminate write amplification 消除写放大
  don't send whole page, just written bytes 不发送整个page，只发送写的bytes

```
Big idea: write diffs 写diffs
  on M1 write fault: 当M1写完，告诉其他host失效副本，但是保留原本
    tell other hosts to invalidate but keep hidden copy
    M1 makes hidden copy as well
  on M2 fault:
    M2 asks M1 for recent modifications  M2请求M1最近的修改
    M1 "diffs" current page against hidden copy M1创建diffs
    M1 send diffs to M2 (and all machines w/ copy of this page)
    M2 applies diffs to its hidden copy
    M1 marks page r/o  M1将page标记为r/o read only
```

问题：write diffs是否改变了一致性模型？

最多只有一个writeable副本，所以写是有序的

当copy是readable，没有写，因此不会读到旧数据

readable的copies是最新的，因此不会读到旧数据



Next goal: allow multiple readers+writers

当一个机器写的时候，don't invalidate others

当其他机器读的时候，不会降级writer为r/o read only

multiple *different* copies of a page，哪一页需要reader来读呢？

diffs起作用了：将write合并到同一页

那什么时候发送diffs？

没有invalidations，不会在写的时候发送失效给其他host了

没有page faults，不会page失效然后去获取数据了

那什么触发diffs发送呢？

```
Example 1 (RC and false sharing)
x and y are on the same page
M0: a1 for(...) x++ r1
M1: a2 for(...) y++ r2  a1 print x, y r1   M1的acquire1，等待M0的release
What does RC do?
  M0 and M1 both get cached writeable copy of the page
  during release, each computes diffs against original page,
    and sends them to all copies
  M1's a1 causes it to wait until M0's release
    so M1 will see M0's writes
```

```
Big idea: lazy release consistency (LRC) 只发送diffs给下一个获取锁的，而不是所有人
  only send write diffs to next acquirer of released lock,
    not to everyone
```



```
Example 2 (lazyness)
x and y on same page (otherwise general-approach avoids copy too)
everyone starts with a copy of that page
M0: a1 x=1 r1
M1:           a2 y=1 r2
M2:                     a1 print x r1
What does LRC do? M2只请求M0的diffs，而不会看到y=1的改变
  M2 only asks previous holder of lock 1 for write diffs
  M2 does not see M1's y=1, even tho on same page (so print y would be stale)
```

```
Example 3 (programs don't lock all shared data)
x, y, and z on the same page
M0: x := 7 a1 y = &x r1
M1:                    a1 a2 z = y r2 r1
M2:                                       a2 print *z r2
will M2 print 7?
LRC as described so far in this lecture would *not* print 7!
  M2 will see the pointer in z, but will have stale content in x's memory.
```

上面的例子中z能获取到最新的y的地址，但是不会看到x最新的值



For real programs to work, Treadmarks must provide "causal consistency（因果一致）":

```
when you see a value,当看到个值的时候，要同时看到其他可能影响该值的值
    you also see other values which might have influenced its computation.
  "influenced" means "processor might have read".
```

## How to track which writes influenced a value？

怎么跟踪哪个写影响了一个值？

对于每个release编号— "interval" numbers

Each machine tracks highest write it has seen from each other machine

**a "Vector Timestamp"**

Tag each release with current VT

On acquire, tell previous holder your VT

​	difference indicates which writes need to be sent

```
VTs order writes to same variable by different machines:
M0: a1 x=1 r1  a2 y=9 r2
M1:              a1 x=2 r1
M2:                           a1 a2 z = x + y r2 r1
M2 is going to hear "x=1" from M0, and "x=2" from M1.
  How does M2 know what to do?
```

M2看到了x=1，x=2，那到底x=?呢

这种情况下根据vt可以进行排序



```
Could the VTs for two values of the same variable not be ordered?
M0: a1 x=1 r1
M1:              a2 x=2 r2
M2:                           a1 a2 print x r2 r1
```

这种情况怎么办呢？对于同一个变量的读取，获取同一个锁



1. each shared variable protected by some lock
2. lock before writing a shared variable
    to order writes to same var
    otherwise "latest value" not well defined
3. lock before reading a shared variable
    to get the latest version
4. if no lock for read, guaranteed to see values that
    contributed to the variables you did lock

