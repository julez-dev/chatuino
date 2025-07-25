// Code generated by easyjson for marshaling/unmarshaling. DO NOT EDIT.

package command

import (
	json "encoding/json"
	easyjson "github.com/mailru/easyjson"
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// suppress unused package warning
var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand(in *jlexer.Lexer, out *PrivateMessage) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "badge_info":
			if in.IsNull() {
				in.Skip()
				out.BadgeInfo = nil
			} else {
				in.Delim('[')
				if out.BadgeInfo == nil {
					if !in.IsDelim(']') {
						out.BadgeInfo = make([]Badge, 0, 2)
					} else {
						out.BadgeInfo = []Badge{}
					}
				} else {
					out.BadgeInfo = (out.BadgeInfo)[:0]
				}
				for !in.IsDelim(']') {
					var v1 Badge
					easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand1(in, &v1)
					out.BadgeInfo = append(out.BadgeInfo, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "badges":
			if in.IsNull() {
				in.Skip()
				out.Badges = nil
			} else {
				in.Delim('[')
				if out.Badges == nil {
					if !in.IsDelim(']') {
						out.Badges = make([]Badge, 0, 2)
					} else {
						out.Badges = []Badge{}
					}
				} else {
					out.Badges = (out.Badges)[:0]
				}
				for !in.IsDelim(']') {
					var v2 Badge
					easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand1(in, &v2)
					out.Badges = append(out.Badges, v2)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "bits":
			out.Bits = int(in.Int())
		case "color":
			out.Color = string(in.String())
		case "display_name":
			out.DisplayName = string(in.String())
		case "emotes":
			if in.IsNull() {
				in.Skip()
				out.Emotes = nil
			} else {
				in.Delim('[')
				if out.Emotes == nil {
					if !in.IsDelim(']') {
						out.Emotes = make([]Emote, 0, 2)
					} else {
						out.Emotes = []Emote{}
					}
				} else {
					out.Emotes = (out.Emotes)[:0]
				}
				for !in.IsDelim(']') {
					var v3 Emote
					easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand2(in, &v3)
					out.Emotes = append(out.Emotes, v3)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "id":
			out.ID = string(in.String())
		case "mod":
			out.Mod = bool(in.Bool())
		case "first_msg":
			out.FirstMsg = bool(in.Bool())
		case "paid_amount":
			out.PaidAmount = int(in.Int())
		case "paid_currency":
			out.PaidCurrency = string(in.String())
		case "paid_exponent":
			out.PaidExponent = int(in.Int())
		case "paid_level":
			out.PaidLevel = string(in.String())
		case "paid_is_system_message":
			out.PaidIsSystemMessage = bool(in.Bool())
		case "parent_msg_id":
			out.ParentMsgID = string(in.String())
		case "parent_user_id":
			out.ParentUserID = string(in.String())
		case "parent_user_login":
			out.ParentUserLogin = string(in.String())
		case "parent_display_name":
			out.ParentDisplayName = string(in.String())
		case "parent_msg_body":
			out.ParentMsgBody = string(in.String())
		case "thread_parent_msg_id":
			out.ThreadParentMsgID = string(in.String())
		case "thread_parent_user_login":
			out.ThreadParentUserLogin = string(in.String())
		case "room_id":
			out.RoomID = string(in.String())
		case "channel_user_name":
			out.ChannelUserName = string(in.String())
		case "subscriber":
			out.Subscriber = bool(in.Bool())
		case "tmi_sent_ts":
			if data := in.Raw(); in.Ok() {
				in.AddError((out.TMISentTS).UnmarshalJSON(data))
			}
		case "turbo":
			out.Turbo = bool(in.Bool())
		case "user_id":
			out.UserID = string(in.String())
		case "user_type":
			out.UserType = UserType(in.String())
		case "vip":
			out.VIP = bool(in.Bool())
		case "source_id":
			out.SourceID = string(in.String())
		case "source_room_id":
			out.SourceRoomID = string(in.String())
		case "source_badges":
			if in.IsNull() {
				in.Skip()
				out.SourceBadges = nil
			} else {
				in.Delim('[')
				if out.SourceBadges == nil {
					if !in.IsDelim(']') {
						out.SourceBadges = make([]Badge, 0, 2)
					} else {
						out.SourceBadges = []Badge{}
					}
				} else {
					out.SourceBadges = (out.SourceBadges)[:0]
				}
				for !in.IsDelim(']') {
					var v4 Badge
					easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand1(in, &v4)
					out.SourceBadges = append(out.SourceBadges, v4)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "message":
			out.Message = string(in.String())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand(out *jwriter.Writer, in PrivateMessage) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"badge_info\":"
		out.RawString(prefix[1:])
		if in.BadgeInfo == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v5, v6 := range in.BadgeInfo {
				if v5 > 0 {
					out.RawByte(',')
				}
				easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand1(out, v6)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"badges\":"
		out.RawString(prefix)
		if in.Badges == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v7, v8 := range in.Badges {
				if v7 > 0 {
					out.RawByte(',')
				}
				easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand1(out, v8)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"bits\":"
		out.RawString(prefix)
		out.Int(int(in.Bits))
	}
	{
		const prefix string = ",\"color\":"
		out.RawString(prefix)
		out.String(string(in.Color))
	}
	{
		const prefix string = ",\"display_name\":"
		out.RawString(prefix)
		out.String(string(in.DisplayName))
	}
	{
		const prefix string = ",\"emotes\":"
		out.RawString(prefix)
		if in.Emotes == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v9, v10 := range in.Emotes {
				if v9 > 0 {
					out.RawByte(',')
				}
				easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand2(out, v10)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"id\":"
		out.RawString(prefix)
		out.String(string(in.ID))
	}
	{
		const prefix string = ",\"mod\":"
		out.RawString(prefix)
		out.Bool(bool(in.Mod))
	}
	{
		const prefix string = ",\"first_msg\":"
		out.RawString(prefix)
		out.Bool(bool(in.FirstMsg))
	}
	{
		const prefix string = ",\"paid_amount\":"
		out.RawString(prefix)
		out.Int(int(in.PaidAmount))
	}
	{
		const prefix string = ",\"paid_currency\":"
		out.RawString(prefix)
		out.String(string(in.PaidCurrency))
	}
	{
		const prefix string = ",\"paid_exponent\":"
		out.RawString(prefix)
		out.Int(int(in.PaidExponent))
	}
	{
		const prefix string = ",\"paid_level\":"
		out.RawString(prefix)
		out.String(string(in.PaidLevel))
	}
	{
		const prefix string = ",\"paid_is_system_message\":"
		out.RawString(prefix)
		out.Bool(bool(in.PaidIsSystemMessage))
	}
	{
		const prefix string = ",\"parent_msg_id\":"
		out.RawString(prefix)
		out.String(string(in.ParentMsgID))
	}
	{
		const prefix string = ",\"parent_user_id\":"
		out.RawString(prefix)
		out.String(string(in.ParentUserID))
	}
	{
		const prefix string = ",\"parent_user_login\":"
		out.RawString(prefix)
		out.String(string(in.ParentUserLogin))
	}
	{
		const prefix string = ",\"parent_display_name\":"
		out.RawString(prefix)
		out.String(string(in.ParentDisplayName))
	}
	{
		const prefix string = ",\"parent_msg_body\":"
		out.RawString(prefix)
		out.String(string(in.ParentMsgBody))
	}
	{
		const prefix string = ",\"thread_parent_msg_id\":"
		out.RawString(prefix)
		out.String(string(in.ThreadParentMsgID))
	}
	{
		const prefix string = ",\"thread_parent_user_login\":"
		out.RawString(prefix)
		out.String(string(in.ThreadParentUserLogin))
	}
	{
		const prefix string = ",\"room_id\":"
		out.RawString(prefix)
		out.String(string(in.RoomID))
	}
	{
		const prefix string = ",\"channel_user_name\":"
		out.RawString(prefix)
		out.String(string(in.ChannelUserName))
	}
	{
		const prefix string = ",\"subscriber\":"
		out.RawString(prefix)
		out.Bool(bool(in.Subscriber))
	}
	{
		const prefix string = ",\"tmi_sent_ts\":"
		out.RawString(prefix)
		out.Raw((in.TMISentTS).MarshalJSON())
	}
	{
		const prefix string = ",\"turbo\":"
		out.RawString(prefix)
		out.Bool(bool(in.Turbo))
	}
	{
		const prefix string = ",\"user_id\":"
		out.RawString(prefix)
		out.String(string(in.UserID))
	}
	{
		const prefix string = ",\"user_type\":"
		out.RawString(prefix)
		out.String(string(in.UserType))
	}
	{
		const prefix string = ",\"vip\":"
		out.RawString(prefix)
		out.Bool(bool(in.VIP))
	}
	{
		const prefix string = ",\"source_id\":"
		out.RawString(prefix)
		out.String(string(in.SourceID))
	}
	{
		const prefix string = ",\"source_room_id\":"
		out.RawString(prefix)
		out.String(string(in.SourceRoomID))
	}
	{
		const prefix string = ",\"source_badges\":"
		out.RawString(prefix)
		if in.SourceBadges == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v11, v12 := range in.SourceBadges {
				if v11 > 0 {
					out.RawByte(',')
				}
				easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand1(out, v12)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"message\":"
		out.RawString(prefix)
		out.String(string(in.Message))
	}
	out.RawByte('}')
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v PrivateMessage) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand(w, v)
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *PrivateMessage) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand(l, v)
}
func easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand2(in *jlexer.Lexer, out *Emote) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "id":
			out.ID = string(in.String())
		case "start":
			out.Start = int(in.Int())
		case "end":
			out.End = int(in.Int())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand2(out *jwriter.Writer, in Emote) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"id\":"
		out.RawString(prefix[1:])
		out.String(string(in.ID))
	}
	{
		const prefix string = ",\"start\":"
		out.RawString(prefix)
		out.Int(int(in.Start))
	}
	{
		const prefix string = ",\"end\":"
		out.RawString(prefix)
		out.Int(int(in.End))
	}
	out.RawByte('}')
}
func easyjson36cb9bedDecodeGithubComJulezDevChatuinoTwitchCommand1(in *jlexer.Lexer, out *Badge) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "name":
			out.Name = string(in.String())
		case "version":
			out.Version = int(in.Int())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson36cb9bedEncodeGithubComJulezDevChatuinoTwitchCommand1(out *jwriter.Writer, in Badge) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"name\":"
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	{
		const prefix string = ",\"version\":"
		out.RawString(prefix)
		out.Int(int(in.Version))
	}
	out.RawByte('}')
}
