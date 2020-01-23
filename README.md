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
   eventbridge-cli - AWS EventBridge cli

USAGE:
   eventbridge [global options] command [command options] [arguments...]

VERSION:
   0.1.0

AUTHOR:
   matteo ridolfi

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --eventbusname value, -b value  EventBridge Bus Name (default: "default")
   --eventpattern value, -e value  EventBridge event pattern (default: "{\"source\": [{\"anything-but\": [\"eventbridge-cli\"]}]}")
   --prettyjson, -j                Pretty JSON output (default: false)
   --help, -h                      show help (default: false)
   --version, -v                   print the version (default: false)
```

### Usage example:
```sh
AWS_PROFILE=myawsprofile eventbridge-cli

AWS_PROFILE=myawsprofile eventbridge-cli -j \
	-b fishnchips-eventbus \
	-e '{"source":["gamma"],"detail":{"channel":["web"]}}'
```

![screenshot](assets/screenshot.png)

