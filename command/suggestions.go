package command

var ModeratorSuggestions = [...]string{
	"/ban <user> [reason]",
	`/ban_selected {{ if .SelectedDisplayName }}{{ .SelectedDisplayName }}{{ else }}<user>{{ end }} [reason]`,
	"/unban <user>",
	`/unban_selected {{ if .SelectedDisplayName }}{{ .SelectedDisplayName }}{{ else }}<user>{{ end }}`,
	"/timeout <username> [duration] [reason]",
	`/timeout_selected {{ if .SelectedDisplayName }}{{ .SelectedDisplayName }}{{ else }}<user>{{ end }} [duration] [reason]`,
	"/delete_all_messages",
	`/delete_selected_message {{ if .MessageID }}{{ .MessageID }}{{ else }}<message_id>{{ end }}`,
	"/announcement <blue|green|orange|purple|primary> <message>",
	"/announcement blue <message>",
	"/announcement green <message>",
	"/announcement orange <message>",
	"/announcement purple <message>",
	"/announcement primary <message>",
	"/marker [description]",
}

var CommandSuggestions = [...]string{
	"/inspect <username>",
	"/popupchat",
	"/channel",
	"/pyramid <word> <count>",
	"/localsubscribers",
	"/localsubscribersoff",
	"/uniqueonly",
	"/uniqueonlysoff",
	"/createclip",
	"/emotes",
}
