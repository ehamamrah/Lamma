package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/admission"
	"github.com/ehamamrah/lamma/backend/internal/chat"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/moderation"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/ratelimit"
	"github.com/ehamamrah/lamma/backend/internal/realtime"
)

// policySet names every rate-limit policy (PLAN.md §10).
type policySet struct {
	create, lookup, joinIP, joinMeeting, password, token, chat, chatMeeting, moderation ratelimit.Policy
}

func (c *container) policies() policySet {
	l := c.cfg.Limits
	p := func(name string, r config.Rate, failClosed bool) ratelimit.Policy {
		return ratelimit.Policy{Name: name, Rate: r, FailClosed: failClosed}
	}
	return policySet{
		create: p("create", l.Create, true), lookup: p("lookup", l.Lookup, false),
		joinIP: p("join_ip", l.JoinIP, false), joinMeeting: p("join_meeting", l.JoinMeeting, false),
		password: p("password", l.Password, true), token: p("token", l.Token, false),
		chat: p("chat", l.Chat, false), chatMeeting: p("chat_meeting", l.ChatMeeting, false),
		moderation: p("moderation", l.Moderation, false),
	}
}

func (c *container) limit(p ratelimit.Policy, key ratelimit.KeyFunc) Middleware {
	return c.limiter.Middleware(p, key)
}

// bySession keys on the caller's participant ID (routes behind Require).
func bySession(r *http.Request) string {
	if p, ok := access.FromContext(r.Context()); ok {
		return p.ParticipantID.String()
	}
	return ratelimit.ByIP(r)
}

// sessionlessMeetingIP keys on meeting+IP for callers without a session, so
// neither a few abusive IPs nor others' joins can block a meeting's guests,
// and session holders resuming are never limited (review SP4-4).
func sessionlessMeetingIP(r *http.Request) string {
	if _, ok := access.FromContext(r.Context()); ok {
		return ""
	}
	return chi.URLParam(r, "code") + "|" + ratelimit.ByIP(r)
}

func (c *container) routes() routes {
	pol := c.policies()
	meetingH := meeting.NewHandler(c.meetings, meeting.HandlerConfig{
		Cookies: c.cookies, PublicBaseURL: c.cfg.PublicBaseURL, OriginHashKey: c.cfg.OriginHashKey,
		CreateLimit: c.limit(pol.create, ratelimit.ByIP), LookupLimit: c.limit(pol.lookup, ratelimit.ByIP),
	})
	partH := participant.NewHandler(c.parts, c.seats, participant.HandlerConfig{
		Cookies: c.cookies, OriginHashKey: c.cfg.OriginHashKey, LiveKitURL: c.cfg.LiveKit.PublicURL,
		JoinIPLimit: c.limit(pol.joinIP, ratelimit.ByIP), JoinMtgLimit: c.limit(pol.joinMeeting, sessionlessMeetingIP),
		TokenLimit: c.limit(pol.token, bySession),
	})
	return routes{
		root: []func(chi.Router){meetingH.RootRoutes},
		scoped: []func(chi.Router){
			meetingH.Routes, partH.Routes,
			admission.NewHandler(c.hub, c.parts).Routes,
			chat.NewHandler(c.chat, c.limit(pol.chat, bySession)).Routes,
			moderation.NewHandler(c.moderation, c.limit(pol.moderation, bySession)).Routes,
		},
		internal: c.internalRoutes,
		resolver: access.NewResolver(c.tx),
	}
}

func (c *container) internalRoutes(r chi.Router) {
	lk := c.cfg.LiveKit
	presence := participant.NewPresence(c.partStore, c.meetings)
	r.Post("/livekit/webhook", realtime.NewWebhookHandler(lk.APIKey, lk.APISecret, presence, c.meetings, c.lk).ServeHTTP)
}
