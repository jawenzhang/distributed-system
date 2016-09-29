package mapreduce

import (
	"os"
	"log"
	"encoding/json"
)

// doReduce does the job of a reduce worker: it reads the intermediate
// key/value pairs (produced by the map phase) for this task, sorts the
// intermediate key/value pairs by key, calls the user-defined reduce function
// (reduceF) for each key, and writes the output to disk.
func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTaskNumber int, // which reduce task this is
	nMap int, // the number of map tasks that were run ("M" in the paper)
	reduceF func(key string, values []string) string,
) {
	// TODO:
	// You will need to write this function.
	// You can find the intermediate file for this reduce task from map task number
	// m using reduceName(jobName, m, reduceTaskNumber).
	// Remember that you've encoded the values in the intermediate files, so you
	// will need to decode them. If you chose to use JSON, you can read out
	// multiple decoded values by creating a decoder, and then repeatedly calling
	// .Decode() on it until Decode() returns an error.
	//
	// You should write the reduced output in as JSON encoded KeyValue
	// objects to a file named mergeName(jobName, reduceTaskNumber). We require
	// you to use JSON here because that is what the merger than combines the
	// output from all the reduce tasks expects. There is nothing "special" about
	// JSON -- it is just the marshalling format we chose to use. It will look
	// something like this:
	//
	// enc := json.NewEncoder(mergeFile)
	// for key in ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()
	// 实现
	// 1. 检查文件是否存在，并打开文件
	keyValues := map[string][]string{}
	for i:=0;i<nMap;i++ {
		intermediateName := reduceName(jobName,i,reduceTaskNumber)
		//log.Println("reduce intermediateFile:",intermediateName)
		if _, err := os.Stat(intermediateName); os.IsNotExist(err) {
			debug("file:%s not exist\n",intermediateName)
			continue
		}
		file, err := os.Open(intermediateName)
		if err != nil {
			log.Fatal("Open file error: ",err)
		}

		dec := json.NewDecoder(file)

		var value KeyValue
		for {
			err := dec.Decode(&value)
			if err != nil {
				break
			}
			if arr, exists := keyValues[value.Key]; exists {
				keyValues[value.Key] = append(arr,value.Value)
			}else {
				keyValues[value.Key] = []string{value.Value}
			}
		}
		file.Close()
	}
	mergeFileName := mergeName(jobName,reduceTaskNumber)
	//log.Println("mergeFileName:",mergeFileName)
	mergeFile,err  := os.Create(mergeFileName)
	if err != nil {
		log.Fatal("Create file err: ",err)
	}
	defer mergeFile.Close()
	enc := json.NewEncoder(mergeFile)
	for key,values := range keyValues {
		err := enc.Encode(&KeyValue{
			Key:key,
			Value:reduceF(key,values),
		})
		if err != nil {
			log.Fatal("encode error: ",err)
		}
	}
}
