package raftkv

import "labrpc"
import "crypto/rand"
import (
	"math/big"
	//"time"
	//"time"
	//"time"
	"sync"
)


type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	id int64
	reqId int64
	mu           sync.Mutex

}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.
	ck.id = nrand()
	ck.reqId = 0
	return ck
}

//
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("RaftKV.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {
	args := GetArgs{Key:key,Id:ck.id}
	ck.mu.Lock()
	args.ReqId = ck.reqId
	ck.reqId++
	ck.mu.Unlock()
	for {
		for _,c := range ck.servers {
			//time.Sleep(time.Millisecond*2000)
			reply := GetReply{}
			ok := c.Call("RaftKV.Get", &args, &reply)
			if ok && !reply.WrongLeader {
				return reply.Value
			}
		}
	}
	// You will have to modify this function.
	return ""
}

//
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("RaftKV.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	//DPrintf("call ")
	args := PutAppendArgs{Key:key,Value:value,Op:op,Id:ck.id}
	ck.mu.Lock()
	args.ReqId = ck.reqId
	ck.reqId++
	ck.mu.Unlock()
	for {
		for _,c := range ck.servers {
			//time.Sleep(time.Millisecond*2000)

			reply := PutAppendReply{}
			ok := c.Call("RaftKV.PutAppend", &args, &reply)
			if ok && !reply.WrongLeader {
				return
			}
		}
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
