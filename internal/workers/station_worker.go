
package workers

import (
"context"
"log"

"github.com/segmentio/kafka-go"
)

func Start() {

reader := kafka.NewReader(kafka.ReaderConfig{
Brokers: []string{"localhost:9092"},
Topic: "station-jobs",
GroupID: "station-workers",
})

for {

msg, err := reader.ReadMessage(context.Background())

if err != nil {
log.Println(err)
continue
}

log.Println("processing station job:", string(msg.Value))

}

}
