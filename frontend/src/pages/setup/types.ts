import type { SetupErrorCode } from '@/lib/setup/validation';

export interface SetupFormState {
  username: string;
  password: string;
  confirm: string;
  port: number | null;
  basePath: string;
  language: string;
}

export type FieldName = 'username' | 'password' | 'port' | 'basePath';
export type FieldErrors = Partial<Record<FieldName, SetupErrorCode>>;
export type PortStatus = 'idle' | 'checking' | 'free' | 'used' | 'unknown';
