package console

import (
	"context"
	"fmt"
	"net/http"
)

// An account's own personal invite code, who has joined through it, and the
// reward it has earned — the terminal counterpart to the invites panel in
// settings. Every route is /api/profile/invites*, never /api/admin/*, so
// Permission: Anyone is correct the same way it is in cmd_self_keys.go: an
// account can only ever see and regenerate its own code.

// inviteesTable renders the "invitees" array both `me invite` and
// `me invite regenerate` answer with, in the same shape.
func inviteesTable(rt *Runtime, invitees []any) {
	fmt.Fprintln(rt.Out, "\ninvitees:")
	rows := make([][]string, 0, len(invitees))
	for _, raw := range invitees {
		u := asMap(raw)
		rewarded := "no"
		if asBoolVal(u["rewarded"]) {
			rewarded = "yes"
		} else if s := asStr(u["reward_skipped"]); s != "" {
			rewarded = s
		}
		rows = append(rows, []string{asStr(u["username"]), asStr(u["nickname"]), formatMS(u["created_at"]), rewarded})
	}
	RenderTable(rt.Out, rt.Session.Width, rt.Session.Colour, []string{"username", "nickname", "joined", "rewarded"}, rows)
}

// printInvitePayload renders the one shape both routes answer with — see
// internal/invite/http.go's own payload comment for why GET and regenerate
// share it. jsonMode's own Fields call already prints the server's raw body
// when --json is in effect, so the invitees table below is skipped then,
// the same way `group show` and `user show` skip their own trailing tables.
func printInvitePayload(rt *Runtime, data any) error {
	m := asMap(data)
	jsonMode := rt.effectiveJSON()

	code := asStr(m["code"])
	link := "-"
	if code != "" {
		// No location.origin in a terminal — this is the path a browser's
		// partner link would carry, for whoever is copying it into a chat
		// or an email themselves.
		link = "/register?invite=" + code
	} else {
		code = "-"
	}
	limit := "unlimited"
	if n := asNum(m["limit"]); n != 0 {
		limit = fmt.Sprint(n)
	}

	if err := rt.Fields([][2]string{
		{"enabled", yesNo(asBoolVal(m["enabled"]))},
		{"code", displayInviteCode(code)},
		{"link", link},
		{"used", fmt.Sprint(asNum(m["used"]))},
		{"limit", limit},
		{"reward_cards", fmt.Sprint(asNum(m["reward_cards"]))},
		{"reward_card_days", fmt.Sprint(asNum(m["reward_card_days"]))},
	}); err != nil {
		return err
	}
	if jsonMode {
		return nil
	}
	inviteesTable(rt, asSlice(m["invitees"]))
	return nil
}

func init() {
	registerCommand(Command{
		Name:    "me invite",
		Group:   "profile",
		Summary: Text{EN: "Show your personal invite code and who joined through it", ZH: "查看你的个人邀请码及通过它加入的人"},
		Usage:   "me invite",
		Help: Text{
			EN: "Only shows a code when the operator has turned personal invite codes on. A code is " +
				"created automatically the first time this is run, if the account does not have a live " +
				"one yet — the same as opening the invites panel in settings does. \"used\" counts " +
				"successful registrations toward \"limit\" (unlimited when 0); the code itself keeps " +
				"working past the limit for other accounts' purposes, it just stops crediting you.",
			ZH: "只有在运营方开启了个人邀请码时才会显示邀请码。若账户还没有有效的邀请码，第一次执行本命令会" +
				"自动创建一个——与打开设置中的邀请码面板效果相同。“used” 是计入“limit”的成功注册数（0 表示" +
				"不限）；超过上限后邀请码本身仍可被使用，只是不再为你计数。",
		},
		Examples:   []string{"me invite", "me invite --json"},
		SeeAlso:    []string{"me invite regenerate"},
		Permission: Anyone,
		Endpoints:  []string{"GET /api/profile/invites"},
		Run: func(_ context.Context, rt *Runtime) error {
			data, _, err := rt.Call(http.MethodGet, "/api/profile/invites", nil)
			if err != nil {
				return err
			}
			return printInvitePayload(rt, data)
		},
	})

	registerCommand(Command{
		Name:    "me invite regenerate",
		Group:   "profile",
		Summary: Text{EN: "Replace your personal invite code with a new one", ZH: "为你换发一个新的个人邀请码"},
		Usage:   "me invite regenerate --yes",
		Help: Text{
			EN: "The old code stops working immediately — anyone holding a link built from it can no " +
				"longer register through it. Registrations it already brought in, and any reward " +
				"already granted for them, are not affected.",
			ZH: "旧邀请码立即失效——任何人拿着用它生成的链接都无法再用它注册。它此前已经带来的注册，" +
				"以及已经发放的奖励，都不受影响。",
		},
		Examples:    []string{"me invite regenerate --yes", "me invite regenerate -y"},
		SeeAlso:     []string{"me invite"},
		Permission:  Anyone,
		Destructive: true,
		Endpoints:   []string{"POST /api/profile/invites/regenerate"},
		Run: func(_ context.Context, rt *Runtime) error {
			data, _, err := rt.Call(http.MethodPost, "/api/profile/invites/regenerate", nil)
			if err != nil {
				return err
			}
			return printInvitePayload(rt, data)
		},
	})
}
