package mr

import "fmt"
import "log"
import "net/rpc"
import "hash/fnv"


//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
	for {
		// 循环请求
		replyMsg := CallForTask()

		switch replyMsg.MsgType {
		case MapTaskAlloc:
			err := HandleMapTask(replyMsg, mapf)
			if err == nil {
				_ = CallForReportStatus(MapSuccess, replyMsg.TaskID)
			} else {
				// log.Println("Worker: Map Task failed")
				_ = CallForReportStatus(MapFailed, replyMsg.TaskID)
			}
		case ReduceTaskAlloc:
			err := HandleReduceTask(replyMsg, reducef)
			if err == nil {
				_ = CallForReportStatus(ReduceSuccess, replyMsg.TaskID)
			} else {
				// log.Println("Worker: Map Task failed")
				_ = CallForReportStatus(ReduceFailed, replyMsg.TaskID)
			}
		case Wait:
			time.Sleep(time.Second * 10)
		case Shutdown:
			os.Exit(0)
		}
		time.Sleep(time.Second)
	}

}
//map处理函数
func HandleMapTask(reply *MessageReply, mapf func(string, string) []KeyValue) error {
	file, err := os.Open(reply.TaskName)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// 进行mapf
	kva := mapf(reply.TaskName, string(content))
	sort.Sort(ByKey(kva))

	oname_prefix := "mr-out-" + strconv.Itoa(reply.TaskID) + "-"

	key_group := map[string][]string{}
	for _, kv := range kva {
		key_group[kv.Key] = append(key_group[kv.Key], kv.Value)
	}

	// 先清理可能存在的垃圾
	// TODO: 原子重命名的方法
	_ = DelFileByMapId(reply.TaskID, "./")

	for key, values := range key_group {
		redId := ihash(key)
		oname := oname_prefix + strconv.Itoa(redId%reply.NReduce)
		var ofile *os.File
		if _, err := os.Stat(oname); os.IsNotExist(err) {
			// 文件夹不存在
			ofile, _ = os.Create(oname)
		} else {
			ofile, _ = os.OpenFile(oname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		enc := json.NewEncoder(ofile)
		for _, value := range values {
			err := enc.Encode(&KeyValue{Key: key, Value: value})
			if err != nil {
				ofile.Close()
				return err
			}
		}
		ofile.Close()
	}
	return nil
}
//删除脏文件
func DelFileByMapId(targetNumber int, path string) error {
	// 创建正则表达式，X 是可变的指定数字
	pattern := fmt.Sprintf(`^mr-out-%d-\d+$`, targetNumber)
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	// 读取当前目录中的文件
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// 遍历文件，查找匹配的文件
	for _, file := range files {
		if file.IsDir() {
			continue // 跳过目录
		}
		fileName := file.Name()
		if regex.MatchString(fileName) {
			// 匹配到了文件，删除它
			filePath := filepath.Join(path, file.Name())
			err := os.Remove(filePath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}


//reduce处理函数
func HandleReduceTask(reply *MessageReply, reducef func(string, []string) string) error {
	key_id := reply.TaskID

	k_vs := map[string][]string{}

	fileList, err := ReadSpecificFile(key_id, "./")

	if err != nil {
		return err
	}

	// 整理所有的中间文件
	for _, file := range fileList {
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			k_vs[kv.Key] = append(k_vs[kv.Key], kv.Value)
		}
		file.Close()
	}

	// 获取所有的键并排序
	var keys []string
	for k := range k_vs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	oname := "mr-out-" + strconv.Itoa(reply.TaskID)
	ofile, err := os.Create(oname)
	if err != nil {
		return err
	}
	defer ofile.Close()

	for _, key := range keys {
		output := reducef(key, k_vs[key])
		_, err := fmt.Fprintf(ofile, "%v %v\n", key, output)
		if err != nil {
			return err
		}
	}

	DelFileByReduceId(reply.TaskID, "./")

	return nil
}


//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

func CallForReportStatus(succesType MsgType, taskID int) error {
	args := MessageSend{
        MsgType: succesType,
        TaskID:  taskID,
    }
	err:=call("Coordinator.NoticeResult",&args,nil)
	return nil
}

func CallForTask() *MessageReply{
	args:=MessageSend{
		MsgType: AskForTask,
	}
	reply:=MessageReply{}
	err:=call("Coordinator.AskForTask",&args,&reply)
	if err==nil{
		// fmt.Printf("TaskName %v, NReduce %v, taskID %v\n", reply.TaskName, reply.NReduce, reply.TaskID)
		return &reply
	}else {
		log.Println(err.Error())
		return nil
	}
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
