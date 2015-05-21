# slack-dump
Generate an export of Channel and / or Private Group history and export it as a ZIP file compatible with Slack's import tool.

## Usage

```
$ slack-dump -h

NAME:
   slack-dump - export channel and group history to the Slack export format

USAGE:
   slack-dump [global options] command [command options] [arguments...]

VERSION:
   0.0.1

AUTHOR(S):
   Joe Fitzgerald <jfitzgerald@pivotal.io>

COMMANDS:
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --token, -t 		a Slack API token: (see: https://api.slack.com/web) [$SLACK_API_TOKEN]
   --help, -h		show help
   --version, -v	print the version
```

### Export All Channels And Private Groups

```
$ slack-dump -t=YOURSLACKAPITOKENISHERE
```

### Export Specific Channels And Private Groups

```
$ slack-dump -t=YOURSLACKAPITOKENISHERE channel-name-here privategroup-name-here another-privategroup-name-here
```
