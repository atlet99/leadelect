# LEADer ELECTion

Нода может иметь статус `leader`, `candidate` или `follower`.
Нода `leader` выбирается **только** большинством нод.
Все ноды при старте имеют статус `follower`.

…

## Пример для запуска на локальной машине

- [LEADer ELECTion](#leader-election)
  - [Пример для запуска на локальной машине](#пример-для-запуска-на-локальной-машине)
    - [cfg.yaml](#cfgyaml)
    - [main.go](#maingo)
    - [Сборка и запуск](#сборка-и-запуск)

### cfg.yaml

```yaml
certs:
  ca: certs/ca-cert.pem
  server:
    cert: certs/server-cert.pem
    key: certs/server-key.pem
nodes:
  1:
    addr: 127.0.0.1
    port: 50001
  2:
    addr: 127.0.0.1
    port: 50002
  3:
    addr: 127.0.0.1
    port: 50003
```

### main.go

```go
package main

import (
    "context"
    "fmt"
    "log"
    "log/slog"
    "os"
    "os/signal"
    "strconv"
    "syscall"
    "time"

    "github.com/atlet99/leadelect/node"
    "gopkg.in/yaml.v3"
)

type config struct {
    Certs struct {
        Ca     string `yaml:"ca"`
        Server struct {
            Cert string `yaml:"cert"`
            Key  string `yaml:"key"`
        } `yaml:"server"`
    }
    Nodes map[string]struct {
        Addr string `yaml:"addr"`
        Port string `yaml:"port"`
    } `yaml:"nodes"`
}

func main() {
    // Load configuration
    b, err := os.ReadFile("cfg.yaml")
    if err != nil {
        slog.Error("failed to read configuration file", "error", err)
        os.Exit(1)
    }

    var cfg config
    if err = yaml.Unmarshal(b, &cfg); err != nil {
        slog.Error("failed to parse configuration file", "error", err)
        os.Exit(1)
    }

    // Ensure node ID is provided as an argument
    if len(os.Args) < 2 {
        slog.Error("node ID argument missing")
        os.Exit(1)
    }

    // Initialize the current node
    nodeID := os.Args[1]
    p, ok := cfg.Nodes[nodeID]
    if !ok {
        slog.Error("node not found in configuration", "nodeID", nodeID)
        os.Exit(1)
    }

    port, err := strconv.Atoi(p.Port)
    if err != nil {
        slog.Error("invalid port number", "nodeID", nodeID, "port", p.Port, "error", err)
        os.Exit(1)
    }

    opts := []node.NodeOpt{
        node.ClientTimeout(time.Second * 10),
        node.HeartBeatTimeout(time.Second * 3),
        node.CheckElectionTimeout(time.Second * 10),
    }

    n := node.New(nodeID, p.Addr, port, opts...)

    // Setup TLS if specified in configuration
    if cfg.Certs.Ca != "" {
        if err = n.ClientTLS(cfg.Certs.Ca, p.Addr); err != nil {
            slog.Error("failed to set up client TLS", "error", err)
            os.Exit(1)
        }
    }
    if cfg.Certs.Server.Cert != "" {
        if err = n.ServerTLS(cfg.Certs.Server.Cert, cfg.Certs.Server.Key); err != nil {
            slog.Error("failed to set up server TLS", "error", err)
            os.Exit(1)
        }
    }

    // Add other nodes
    for id, v := range cfg.Nodes {
        if id == nodeID {
            continue
        }
        otherPort, err := strconv.Atoi(v.Port)
        if err != nil {
            slog.Warn("invalid port for node, skipping", "nodeID", id, "port", v.Port, "error", err)
            continue
        }
        n.AddNode(node.New(id, v.Addr, otherPort))
    }

    // Start the node
    ctx, cancel := context.WithCancel(context.Background())
    go n.Run(ctx, slog.Default())

    // Periodically log the status of the node
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    go func() {
        for range ticker.C {
            slog.Info("Node status", "nodeID", nodeID, "status", n.Status())
        }
    }()

    // Handle graceful shutdown
    interrupt := make(chan os.Signal, 1)
    signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

    <-interrupt
    slog.Info("Shutting down node", "nodeID", nodeID)
    cancel()
    time.Sleep(3 * time.Second) // Wait for graceful shutdown
    slog.Info("Shutdown complete", "nodeID", nodeID)
}
```

### Сборка и запуск

Сборка:

```shell
go build -o app
```

Сгенерировать сертификаты:

```shell
./gen_cert.sh
```

Запуск в разных консолях:

```shell
./app 1
./app 2
./app 3
```
