# 6.5840(原6.824)-2023

## lab1 

1. 主要实现`master`和`worker`之间的`rpc`通信。
2. `worker`向`master`发起`rpc`请求任务，对`maptask` `reducetask`分别进行处理，将处理结果通过`rpc`报告给`master`
3. `master`记录`worker`的状态信息，访问`master`需要加锁，可以按照`map` `reduce`任务分开加锁，也可以加读写锁来替换大锁
4. 使用原子重命名避免竞争

