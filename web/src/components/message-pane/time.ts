export function formatTime(value: string): string {
  return formatMessageTimestamp(value);
}

export function formatMessageTimestamp(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) {
    return value;
  }
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()} ${padTimePart(
    date.getHours()
  )}:${padTimePart(date.getMinutes())}`;
}

function padTimePart(value: number): string {
  return String(value).padStart(2, "0");
}
