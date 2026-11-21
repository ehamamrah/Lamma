export type Role = 'host' | 'guest';
export type Status = 'waiting' | 'admitted' | 'denied' | 'removed' | 'left';

export interface MeetingInfo {
  code: string;
  url?: string;
  title: string;
  requiresPassword: boolean;
  waitingRoom: boolean;
  locked: boolean;
  expiresAt: string;
}

export interface Participant {
  id: string;
  displayName: string;
  role: Role;
  status: Status;
}

export interface TokenGrant {
  token: string;
  url: string;
  expiresAt: string;
}

export interface ChatMessage {
  id: string;
  participantId: string;
  displayName: string;
  text: string;
  sentAt: string;
}

export interface CreateMeetingInput {
  displayName: string;
  title?: string;
  password?: string;
  waitingRoom: boolean;
  expiresInMinutes?: number;
}

export type MuteSource = 'microphone' | 'camera' | 'screen_share';
