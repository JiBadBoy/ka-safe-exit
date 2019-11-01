##并发安全退出
1. 如何通知多个Goroutine？

    - 我们可以通知`close`关闭一个管道来实现广播的效果，
    所有从关闭管道接收的操作均会收到一个零值和一个可选的失败标志。
    ```go
    package main
    import (
       "fmt"
       "time"
    )
    func worker(stopChan chan bool) {
       for{
           select{
           default:
               fmt.Println("hello") //正常工作
           case <- stopChan:
               fmt.Println("exit worker") // 退出
           }   
       }   
    }
   
    func main() {
       stopChan := make(chan bool)
       
       for i:=0; i < 10; i++ {
           go worker(stopChan)
       }
       time.Sleep(time.Second)
       close(stopChan)
    }
    ```
    - 不过这个程序依然不健壮：当每个Goroutine收到退出指令时一般会进行一定的清理工作，但是退出的清理工作并不能保证被完成，
    因为`main`线程并没有等待各个工作Goroutine退出工作完成的机制。我们可以结合`sync.WaitGroup`来改进
    ```go
    package main
    import (   
    "fmt"
    "sync"
    "time"
    )
    func worker(stopChan chan bool, wg sync.WaitGroup) {
       defer wg.Done()
       for{
           select{
           default:
               fmt.Println("hello") //正常工作
           case <- stopChan:
               fmt.Println("exit worker") // 退出
           }   
       }   
     }
       
    func main() {
       stopChan := make(chan bool)
       var wg sync.WaitGroup
       for i:=0; i < 10; i++ {
           wg.Add(1)
           go worker(stopChan, wg)
       }
       time.Sleep(time.Second)
       close(stopChan)
       wg.Wait()
    }
    ```
    - 现在每个工作并发体的创建、运行、暂停和退出都是在`mian`函数的安全控制之下了。
2. context包
    - 在Go1.7发布时，标准库增加了一个`context`包，
    用来简化对于处理单个请求的多个Goroutine之间与请求域的数据、超时和退出等操作。
    我们可以用context包来重新实现前面的线程安全退出或超时控制
    ```go
    package main
    import ( 
    "context"  
    "fmt"
    "sync"
    "time"
    )
    func worker(ctx context.Context, wg sync.WaitGroup) {
       defer wg.Done()
       for{
           select{
           default:
               fmt.Println("hello") //正常工作
           case <- ctx.Done():
               fmt.Println("exit worker") // 退出
           }   
       }   
     }
       
    func main() {
       ctx, cancel := context.WithCancel(context.Background())
       var wg sync.WaitGroup
       for i:=0; i < 10; i++ {
           wg.Add(1)
           go worker(ctx, wg)
       }
       time.Sleep(time.Second)
       cancel()
       wg.Wait()
    }
    ```
   - 当`main`主动停止工作者Goroutine时，每个工作者都可以安全退出
   
3. kafka的优雅退出

    - kafka的退出，我们也可以用上述的方法来实现安全退出，具体实现方式请查看`consumer/consumer.go`