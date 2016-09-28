package mapreduce

import (
	"hash/fnv"
	"os"
	"log"
	"encoding/json"
)

// doMap does the job of a map worker: it reads one of the input files
// (inFile), calls the user-defined map function (mapF) for that file's
// contents, and partitions the output into nReduce intermediate files.
func doMap(
jobName string, // the name of the MapReduce job
mapTaskNumber int, // which map task this is
inFile string,
nReduce int, // the number of reduce task that will be run ("R" in the paper)
mapF func(file string, contents string) []KeyValue,
) {
	// TODO:
	// You will need to write this function.
	// You can find the filename for this map task's input to reduce task number
	// r using reduceName(jobName, mapTaskNumber, r). The ihash function (given
	// below doMap) should be used to decide which file a given key belongs into.
	//
	// The intermediate output of a map task is stored in the file
	// system as multiple files whose name indicates which map task produced
	// them, as well as which reduce task they are for. Coming up with a
	// scheme for how to store the key/value pairs on disk can be tricky,
	// especially when taking into account that both keys and values could
	// contain newlines, quotes, and any other character you can think of.
	//
	// One format often used for serializing data to a byte stream that the
	// other end can correctly reconstruct is JSON. You are not required to
	// use JSON, but as the output of the reduce tasks *must* be JSON,
	// familiarizing yourself with it here may prove useful. You can write
	// out a data structure as a JSON string to a file using the commented
	// code below. The corresponding decoding functions can be found in
	// common_reduce.go.
	//
	//   enc := json.NewEncoder(file)
	//   for _, kv := ... {
	//     err := enc.Encode(&kv)
	//
	// Remember to close the file after you have written all the values!

	// my implement
	// 1. 打开输入文件
	file, err := os.Open(inFile)
	if err != nil {
		log.Fatal("Open file error: ", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatal("Get file info error: ", err)
	}
	size := fileInfo.Size()
	buf := make([]byte, size)
	_, err = file.Read(buf)
	if err != nil {
		log.Fatal("Read file error: ", err)
	}
	file.Close()
	// 2. 处理得到keyValue
	keyValues := mapF(inFile, string(buf))
	// 3. hash得到reducer num
	intermediateDecoders := map[string]*json.Encoder{}
	intermediateFiles := map[string]*os.File{}
	for _, value := range keyValues {
		rN := ihash(value.Key) % nReduce
		intermediateName := reduceName(jobName, mapTaskNumber, rN)
		// 4. 输出到中间文件
		enc, exists := intermediateDecoders[intermediateName]
		if !exists {
			// 打开文件
			file, err := os.Create(intermediateName)
			if err != nil {
				log.Fatal("Open file error: ", err)
			}
			enc := json.NewEncoder(file)
			intermediateDecoders[intermediateName] = enc
			intermediateFiles[intermediateName] = file
		}
		err := enc.Encode(&value)
		if err != nil {
			log.Fatal("Encoder error: ", err)
		}
	}
	for _, file := range intermediateFiles {
		file.Close()
	}
}

func ihash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
