export interface Guest {
  id: string;
  name: string;
}

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}

export interface HostSessionResponse {
  session_id: string;
  link: string;
}

export interface OfferPayload {
  sdp: string;
  guest_id: string;
}

export interface AnswerPayload {
  sdp: string;
  guest_id: string;
}

export interface IcePayload {
  candidate: string;
  guest_id: string;
}

export interface GuestJoinedPayload {
  id: string;
  name: string;
}

export type QualityPreset = 'low' | 'medium' | 'high' | 'ultra';

export interface QualityConfig {
  width: number;
  height: number;
  frameRate: number;
  maxBitrate: number;
}