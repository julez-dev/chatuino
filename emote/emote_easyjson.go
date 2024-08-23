// Code generated by easyjson for marshaling/unmarshaling. DO NOT EDIT.

package emote

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

func easyjsonC0c766a6DecodeGithubComJulezDevChatuinoEmote(in *jlexer.Lexer, out *DecodedImage) {
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
		case "width":
			out.Width = int(in.Int())
		case "height":
			out.Height = int(in.Int())
		case "encoded_path":
			out.EncodedPath = string(in.String())
		case "delay_in_ms":
			out.DelayInMS = int(in.Int())
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
func easyjsonC0c766a6EncodeGithubComJulezDevChatuinoEmote(out *jwriter.Writer, in DecodedImage) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"width\":"
		out.RawString(prefix[1:])
		out.Int(int(in.Width))
	}
	{
		const prefix string = ",\"height\":"
		out.RawString(prefix)
		out.Int(int(in.Height))
	}
	{
		const prefix string = ",\"encoded_path\":"
		out.RawString(prefix)
		out.String(string(in.EncodedPath))
	}
	{
		const prefix string = ",\"delay_in_ms\":"
		out.RawString(prefix)
		out.Int(int(in.DelayInMS))
	}
	out.RawByte('}')
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v DecodedImage) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonC0c766a6EncodeGithubComJulezDevChatuinoEmote(w, v)
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *DecodedImage) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonC0c766a6DecodeGithubComJulezDevChatuinoEmote(l, v)
}
func easyjsonC0c766a6DecodeGithubComJulezDevChatuinoEmote1(in *jlexer.Lexer, out *DecodedEmote) {
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
		case "cols":
			out.Cols = int(in.Int())
		case "images":
			if in.IsNull() {
				in.Skip()
				out.Images = nil
			} else {
				in.Delim('[')
				if out.Images == nil {
					if !in.IsDelim(']') {
						out.Images = make([]DecodedImage, 0, 1)
					} else {
						out.Images = []DecodedImage{}
					}
				} else {
					out.Images = (out.Images)[:0]
				}
				for !in.IsDelim(']') {
					var v1 DecodedImage
					(v1).UnmarshalEasyJSON(in)
					out.Images = append(out.Images, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
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
func easyjsonC0c766a6EncodeGithubComJulezDevChatuinoEmote1(out *jwriter.Writer, in DecodedEmote) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"cols\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Int(int(in.Cols))
	}
	{
		const prefix string = ",\"images\":"
		out.RawString(prefix)
		if in.Images == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v2, v3 := range in.Images {
				if v2 > 0 {
					out.RawByte(',')
				}
				(v3).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v DecodedEmote) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonC0c766a6EncodeGithubComJulezDevChatuinoEmote1(w, v)
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *DecodedEmote) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonC0c766a6DecodeGithubComJulezDevChatuinoEmote1(l, v)
}
