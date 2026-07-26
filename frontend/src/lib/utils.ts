import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** Merges Tailwind class lists, resolving conflicts (shadcn-svelte convention). */
export function cn(...inputs: ClassValue[]): string {
	return twMerge(clsx(inputs));
}

/** Human-readable byte size (1024-based), e.g. "3.2 MB". Used by the admin
 * info screen and anywhere a PDF's size_bytes is displayed. */
export function formatBytes(bytes: number): string {
	if (bytes <= 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const exp = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
	const value = bytes / 1024 ** exp;
	return `${exp === 0 ? value : value.toFixed(1)} ${units[exp]}`;
}

/** Locale-formatted timestamp for created_at/last_viewed_at fields (all
 * RFC3339 strings from the Go backend). */
export function formatDate(iso: string | null | undefined): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleString();
}
