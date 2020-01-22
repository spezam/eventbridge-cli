# ![logo](assets/logo.png) eventbridge-cli

Amazon EventBridge is a serverless event bus that makes it easy to connect applications together using data from your own applications, integrated Software-as-a-Service (SaaS) applications, and AWS services.

Evenbridge-cli is a tool to listen to an EventBus events. Useful for debugging.
```
EventBus --> EventBrige Rule --> SQS <-- poller
```

### Build:
```
go build -o eventbridge-cli
```

### Flags:
```
NAME:
   eventbridge-cli - AWS Eventbridge cli

USAGE:
   eventbridge-cli [global options] command [command options] [arguments...]

VERSION:
   0.0.1

AUTHOR:
   matteo ridolfi

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --eventbusname value, -b value  EventBridge Bus Name (default: "default")
   --prettyjson, -j                Pretty JSON output (default: false)
   --help, -h                      show help (default: false)
   --version, -v                   print the version (default: false)
   ```

### Usage:
```sh
AWS_PROFILE=myawsprofile eventbridge-cli -b fishnchips-eventbus -j
```

![screenshot](assets/screenshot.png)
