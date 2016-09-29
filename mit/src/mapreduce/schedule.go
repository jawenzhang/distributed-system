package mapreduce

import (
	"fmt"
	"sync"
)

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// 总共有ntasks个任务需要给worker，此处使用串行的方式，肯定会非常慢，测试花了3.02s
	// 并行后花费时间2.352s
	var wg sync.WaitGroup
	for i := 0; i < ntasks; i++ {
		go func(i int) {
			wg.Add(1)
			worker := <- mr.registerChannel

			debug("Master: schdule worker %s\n", worker)
			arg := &DoTaskArgs{
				JobName:mr.jobName,
				File:mr.files[i],
				Phase:phase,
				TaskNumber:i,
				NumOtherPhase:nios,
			}
			ok := call(worker, "Worker.DoTask", arg, new(struct{}))
			if ok == false {
				fmt.Printf("Worker: RPC %s DoTask error\n", worker)
			} else {
				go func() {
					mr.registerChannel <- worker
				}()
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	fmt.Printf("Schedule: %v phase done\n", phase)
}
