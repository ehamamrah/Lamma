import { ApiError, request } from './client';
import type {
  ChatMessage,
  CreateMeetingInput,
  MeetingInfo,
  MuteSource,
  Participant,
  TokenGrant,
} from './types';

const m = (code: string) => `/meetings/${code}`;

export const api = {
  createMeeting: (input: CreateMeetingInput) =>
    request<{ meeting: MeetingInfo; participant: Participant }>('POST', '/meetings', input),
  getMeeting: (code: string) => request<MeetingInfo>('GET', m(code)),
  join: (code: string, displayName: string, password?: string) =>
    request<{ participant: Participant }>('POST', `${m(code)}/join`, { displayName, password }),
  token: (code: string) => request<TokenGrant>('POST', `${m(code)}/token`),
  leave: (code: string) => request<void>('POST', `${m(code)}/leave`),
  chatHistory: (code: string) => request<{ messages: ChatMessage[] }>('GET', `${m(code)}/chat`),
  sendChat: (code: string, text: string) =>
    request<ChatMessage>('POST', `${m(code)}/chat`, { text }),

  /** The caller's participant, or null without a session for this meeting. */
  me: async (code: string): Promise<Participant | null> => {
    try {
      return (await request<{ participant: Participant }>('GET', `${m(code)}/me`)).participant;
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return null;
      throw err;
    }
  },

  host: {
    waiting: (code: string) =>
      request<{ participants: Participant[] }>('GET', `${m(code)}/participants?status=waiting`),
    admit: (code: string, id: string) =>
      request<void>('POST', `${m(code)}/participants/${id}/admit`),
    deny: (code: string, id: string) => request<void>('POST', `${m(code)}/participants/${id}/deny`),
    denyAll: (code: string) =>
      request<{ denied: number }>('POST', `${m(code)}/participants/deny-waiting`),
    remove: (code: string, id: string) =>
      request<void>('POST', `${m(code)}/participants/${id}/remove`),
    mute: (code: string, id: string, source: MuteSource) =>
      request<void>('POST', `${m(code)}/participants/${id}/mute`, { source }),
    settings: (code: string, s: { locked?: boolean; waitingRoom?: boolean }) =>
      request<{ locked: boolean; waitingRoom: boolean }>('PATCH', `${m(code)}/settings`, s),
    end: (code: string) => request<void>('POST', `${m(code)}/end`),
  },
};
