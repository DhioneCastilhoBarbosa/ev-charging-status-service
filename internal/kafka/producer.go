
package kafka

import (
"github.com/segmentio/kafka-go"
)

func NewProducer(broker string) *kafka.Writer {

return &kafka.Writer{
Addr: kafka.TCP(broker),
Topic: "station-jobs",
Balancer: &kafka.LeastBytes{},
}

}
