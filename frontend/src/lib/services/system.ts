import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';

export interface AppInfo {
  version: string;
  platform: string;
  arch: string;
  devMock: boolean;
}

export interface SystemInfo {
  os: string;
  arch: string;
  cpu: string;
  cores: number;
  ramBytes: number;
}

export interface LogBundle {
  path: string;
  name: string;
  dir: string;
  sizeBytes: number;
}

export interface StorageInfo {
  path: string;
  volume: string;
  filesystem: string;
  totalBytes: number;
  freeBytes: number;
  usedBytes: number;
}

const GB = 1024 ** 3;

const fixtureStorage: StorageInfo = {
  path: 'D:\\Typhon\\Games',
  volume: 'D:',
  filesystem: 'NTFS',
  totalBytes: 1024 * GB,
  freeBytes: 712 * GB,
  usedBytes: 312 * GB,
};

export async function getAppInfo(): Promise<AppInfo> {
  if (inWails) return (await AppService.GetAppInfo()) as AppInfo;
  return { version: '0.1.0', platform: 'browser', arch: 'dev', devMock: false };
}

export async function getSystemInfo(): Promise<SystemInfo> {
  if (inWails) return (await AppService.GetSystemInfo()) as SystemInfo;
  return { os: 'Browser preview', arch: 'dev', cpu: 'Dev CPU', cores: 8, ramBytes: 16 * GB };
}

export async function getStorageInfo(): Promise<StorageInfo> {
  if (inWails) return (await AppService.GetStorageInfo()) as StorageInfo;
  return fixtureStorage;
}

export async function getStorageInfoFor(path: string): Promise<StorageInfo> {
  if (inWails) return (await AppService.GetStorageInfoFor(path)) as StorageInfo;
  return { ...fixtureStorage, path };
}

export async function exportLogs(): Promise<LogBundle> {
  if (!inWails) throw new Error('unavailable in browser');
  return (await AppService.ExportLogs()) as LogBundle;
}
