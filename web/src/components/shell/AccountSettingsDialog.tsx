import type { Dispatch, SetStateAction } from "react";
import { Download, LogOut, RefreshCw, Save, Send, Server as ServerIcon, Upload } from "lucide-react";
import type { NotificationSettings, Organization, Project, ServerSettings, ToolUpdateOverview, User, UserPreferences } from "@/api/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { BrowserNotificationPermission } from "@/notifications/browser";
import { browserPermissionLabel } from "./utils";

type StringSetter = Dispatch<SetStateAction<string>>;
type BooleanSetter = Dispatch<SetStateAction<boolean>>;

interface AccountSettingsDialogProps {
  user: User;
  organization?: Organization;
  project?: Project;
  accountSettingsOpen: boolean;
  setAccountSettingsOpen: BooleanSetter;
  browserPermission: BrowserNotificationPermission;
  enableBrowserNotifications: () => void | Promise<void>;
  preferences: UserPreferences;
  preferencesLoading: boolean;
  preferencesPending: boolean;
  preferencesError: string | null;
  showTTFT: boolean;
  showTPS: boolean;
  hideAvatars: boolean;
  saveUserPreferences: (next: UserPreferences) => Promise<void>;
  notificationSettings?: NotificationSettings;
  notificationSettingsLoading: boolean;
  webhookEnabled: boolean;
  setWebhookEnabled: BooleanSetter;
  webhookURL: string;
  setWebhookURL: StringSetter;
  webhookSecret: string;
  setWebhookSecret: StringSetter;
  notificationActionError: string | null;
  notificationActionStatus: string | null;
  notificationTestPending: boolean;
  notificationSavePending: boolean;
  sendTestWebhook: () => void | Promise<void>;
  saveNotificationSettings: () => void | Promise<void>;
  serverSettings?: ServerSettings;
  serverSettingsLoading: boolean;
  serverSettingsError: string | null;
  toolUpdates?: ToolUpdateOverview;
  toolUpdatesLoading: boolean;
  toolAutoEnabled: boolean;
  setToolAutoEnabled: BooleanSetter;
  toolTimeOfDay: string;
  setToolTimeOfDay: StringSetter;
  toolTimezone: string;
  setToolTimezone: StringSetter;
  toolClaudeEnabled: boolean;
  setToolClaudeEnabled: BooleanSetter;
  toolCodexEnabled: boolean;
  setToolCodexEnabled: BooleanSetter;
  toolSettingsPending: boolean;
  toolActionStatus: string | null;
  toolActionError: string | null;
  saveToolUpdateSettings: () => void | Promise<void>;
  checkAllToolUpdates: () => void | Promise<void>;
  runAllToolUpdates: () => void | Promise<void>;
  serverListenIP: string;
  setServerListenIP: StringSetter;
  serverListenPort: string;
  setServerListenPort: StringSetter;
  serverTLSEnabled: boolean;
  setServerTLSEnabled: BooleanSetter;
  serverTLSListenPort: string;
  setServerTLSListenPort: StringSetter;
  serverTLSCertFile: string;
  setServerTLSCertFile: StringSetter;
  serverTLSKeyFile: string;
  setServerTLSKeyFile: StringSetter;
  serverTLSCertPEM: string;
  setServerTLSCertPEM: StringSetter;
  serverTLSKeyPEM: string;
  setServerTLSKeyPEM: StringSetter;
  serverListenPortValid: boolean;
  serverTLSListenPortValid: boolean;
  serverPortsConflict: boolean;
  serverTLSCertAvailable: boolean;
  serverTLSKeyAvailable: boolean;
  serverSettingsPending: boolean;
  serverSettingsActionError: string | null;
  serverSettingsActionStatus: string | null;
  serverSettingsSaveDisabled: boolean;
  readServerPEMUpload: (file: File, update: (value: string) => void) => Promise<void>;
  saveServerSettings: () => void | Promise<void>;
  onLogout: () => void;
}

export function AccountSettingsDialog({
  user,
  organization,
  project,
  accountSettingsOpen,
  setAccountSettingsOpen,
  browserPermission,
  enableBrowserNotifications,
  preferences,
  preferencesLoading,
  preferencesPending,
  preferencesError,
  showTTFT,
  showTPS,
  hideAvatars,
  saveUserPreferences,
  notificationSettings,
  notificationSettingsLoading,
  webhookEnabled,
  setWebhookEnabled,
  webhookURL,
  setWebhookURL,
  webhookSecret,
  setWebhookSecret,
  notificationActionError,
  notificationActionStatus,
  notificationTestPending,
  notificationSavePending,
  sendTestWebhook,
  saveNotificationSettings,
  serverSettings,
  serverSettingsLoading,
  serverSettingsError,
  toolUpdates,
  toolUpdatesLoading,
  toolAutoEnabled,
  setToolAutoEnabled,
  toolTimeOfDay,
  setToolTimeOfDay,
  toolTimezone,
  setToolTimezone,
  toolClaudeEnabled,
  setToolClaudeEnabled,
  toolCodexEnabled,
  setToolCodexEnabled,
  toolSettingsPending,
  toolActionStatus,
  toolActionError,
  saveToolUpdateSettings,
  checkAllToolUpdates,
  runAllToolUpdates,
  serverListenIP,
  setServerListenIP,
  serverListenPort,
  setServerListenPort,
  serverTLSEnabled,
  setServerTLSEnabled,
  serverTLSListenPort,
  setServerTLSListenPort,
  serverTLSCertFile,
  setServerTLSCertFile,
  serverTLSKeyFile,
  setServerTLSKeyFile,
  serverTLSCertPEM,
  setServerTLSCertPEM,
  serverTLSKeyPEM,
  setServerTLSKeyPEM,
  serverListenPortValid,
  serverTLSListenPortValid,
  serverPortsConflict,
  serverTLSCertAvailable,
  serverTLSKeyAvailable,
  serverSettingsPending,
  serverSettingsActionError,
  serverSettingsActionStatus,
  serverSettingsSaveDisabled,
  readServerPEMUpload,
  saveServerSettings,
  onLogout,
}: AccountSettingsDialogProps) {
  return (
            <Dialog open={accountSettingsOpen} onOpenChange={setAccountSettingsOpen}>
              <DialogContent className="max-h-[calc(100vh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
                <DialogHeader>
                  <DialogTitle>User settings</DialogTitle>
                  <DialogDescription>Session and workspace details.</DialogDescription>
                </DialogHeader>
                <div className="min-h-0 space-y-4 overflow-y-auto py-2 pr-1" data-testid="user-settings-scroll">
                  <div className="grid gap-3 rounded-md border border-border p-3 text-sm">
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">User</span>
                      <span className="truncate font-medium">{user.display_name}</span>
                    </div>
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">Organization</span>
                      <span className="truncate font-medium">{organization?.name ?? "None"}</span>
                    </div>
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">Project</span>
                      <span className="truncate font-medium">{project?.name ?? "None"}</span>
                    </div>
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-medium">Browser notifications</h3>
                        <p className="text-xs text-muted-foreground">{browserPermissionLabel(browserPermission)}</p>
                      </div>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={enableBrowserNotifications}
                        disabled={browserPermission === "granted" || browserPermission === "denied" || browserPermission === "unsupported"}
                      >
                        Enable
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div>
                      <h3 className="text-sm font-medium">Message layout</h3>
                      <p className="text-xs text-muted-foreground">
                        {preferencesPending ? "Saving" : "Controls chat density"}
                      </p>
                    </div>
                    <label className="flex items-center justify-between gap-4 text-sm">
                      <span>Hide avatars</span>
                      <Switch
                        checked={hideAvatars}
                        disabled={preferencesLoading || preferencesPending}
                        aria-label="Hide avatars"
                        onCheckedChange={(checked) =>
                          void saveUserPreferences({
                            show_ttft: showTTFT,
                            show_tps: showTPS,
                            hide_avatars: checked,
                          })
                        }
                      />
                    </label>
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div>
                      <h3 className="text-sm font-medium">Message metrics</h3>
                      <p className="text-xs text-muted-foreground">
                        {preferencesPending ? "Saving" : "Shown under bot replies"}
                      </p>
                    </div>
                    <label className="flex items-center justify-between gap-4 text-sm">
                      <span>Show TTFT</span>
                      <Switch
                        checked={showTTFT}
                        disabled={preferencesLoading || preferencesPending}
                        aria-label="Show TTFT"
                        onCheckedChange={(checked) =>
                          void saveUserPreferences({
                            show_ttft: checked,
                            show_tps: showTPS,
                            hide_avatars: hideAvatars,
                          })
                        }
                      />
                    </label>
                    <label className="flex items-center justify-between gap-4 text-sm">
                      <span>Show TPS</span>
                      <Switch
                        checked={showTPS}
                        disabled={preferencesLoading || preferencesPending}
                        aria-label="Show TPS"
                        onCheckedChange={(checked) =>
                          void saveUserPreferences({
                            show_ttft: showTTFT,
                            show_tps: checked,
                            hide_avatars: hideAvatars,
                          })
                        }
                      />
                    </label>
                    {preferencesError && <p className="text-sm text-destructive">{preferencesError}</p>}
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-medium">Webhook</h3>
                        <p className="text-xs text-muted-foreground">
                          {notificationSettings?.webhook_secret_configured ? "Secret configured" : "No secret configured"}
                        </p>
                      </div>
                      <label className="flex items-center gap-2 text-sm">
                        <Checkbox
                          checked={webhookEnabled}
                          onChange={(event) => setWebhookEnabled(event.currentTarget.checked)}
                          disabled={notificationSettingsLoading}
                          aria-label="Enable webhook"
                        />
                        Enabled
                      </label>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="webhook-url">URL</Label>
                      <Input
                        id="webhook-url"
                        value={webhookURL}
                        onChange={(event) => setWebhookURL(event.target.value)}
                        placeholder="https://example.com/agentx/${title}/${body}"
                        disabled={notificationSettingsLoading}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="webhook-secret">Secret</Label>
                      <Input
                        id="webhook-secret"
                        value={webhookSecret}
                        onChange={(event) => setWebhookSecret(event.target.value)}
                        placeholder={notificationSettings?.webhook_secret_configured ? "Leave blank to keep current secret" : "Optional signing secret"}
                        disabled={notificationSettingsLoading}
                        type="password"
                      />
                    </div>
                    {(notificationActionError || notificationActionStatus) && (
                      <p className={cn("text-sm", notificationActionError ? "text-destructive" : "text-muted-foreground")}>
                        {notificationActionError ?? notificationActionStatus}
                      </p>
                    )}
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        onClick={sendTestWebhook}
                        disabled={notificationSettingsLoading || notificationTestPending || !webhookURL.trim()}
                      >
                        <Send className="h-4 w-4" />
                        Test
                      </Button>
                      <Button
                        type="button"
                        onClick={saveNotificationSettings}
                        disabled={notificationSettingsLoading || notificationSavePending || (webhookEnabled && !webhookURL.trim())}
                      >
                        <Save className="h-4 w-4" />
                        Save
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="flex items-center gap-2 text-sm font-medium">
                          <ServerIcon className="h-4 w-4" />
                          Server / SSL
                        </h3>
                        <p className="text-xs text-muted-foreground">
                          {serverSettings?.restart_required
                            ? "Saved changes require restart"
                            : `HTTP ${serverSettings?.effective_http_addr ?? serverSettings?.effective_addr ?? "from config"}${serverSettings?.effective_https_addr ? `, HTTPS ${serverSettings.effective_https_addr}` : ""}`}
                        </p>
                      </div>
                      <label className="flex items-center gap-2 text-sm">
                        <Switch
                          checked={serverTLSEnabled}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          aria-label="Enable HTTPS"
                          onCheckedChange={setServerTLSEnabled}
                        />
                        HTTPS
                      </label>
                    </div>
                    {serverSettings?.addr_override_active && (
                      <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                        AGENTX_ADDR is active ({serverSettings.addr_override_value}); HTTP listen IP and port values are saved to config.toml but the environment override controls HTTP startup.
                      </p>
                    )}
                    {serverSettingsError && (
                      <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
                        {serverSettingsError}
                      </p>
                    )}
                    <div className="grid gap-3 sm:grid-cols-[1fr_8rem_8rem]">
                      <div className="space-y-2">
                        <Label htmlFor="server-listen-ip">Listen IP</Label>
                        <Input
                          id="server-listen-ip"
                          value={serverListenIP}
                          onChange={(event) => setServerListenIP(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="server-listen-port">HTTP port</Label>
                        <Input
                          id="server-listen-port"
                          value={serverListenPort}
                          onChange={(event) => setServerListenPort(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          inputMode="numeric"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="server-tls-listen-port">HTTPS port</Label>
                        <Input
                          id="server-tls-listen-port"
                          value={serverTLSListenPort}
                          onChange={(event) => setServerTLSListenPort(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          inputMode="numeric"
                        />
                      </div>
                    </div>
                    {!serverListenPortValid && (
                      <p className="text-xs text-destructive">HTTP port must be between 1 and 65535.</p>
                    )}
                    {!serverTLSListenPortValid && (
                      <p className="text-xs text-destructive">HTTPS port must be between 1 and 65535.</p>
                    )}
                    {serverPortsConflict && (
                      <p className="text-xs text-destructive">HTTP and HTTPS ports must be different.</p>
                    )}
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="server-cert-file">Certificate path</Label>
                        <Input
                          id="server-cert-file"
                          value={serverTLSCertFile}
                          onChange={(event) => setServerTLSCertFile(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          placeholder="/etc/agentx/cert.pem"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="server-key-file">Private key path</Label>
                        <Input
                          id="server-key-file"
                          value={serverTLSKeyFile}
                          onChange={(event) => setServerTLSKeyFile(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          placeholder="/etc/agentx/key.pem"
                        />
                      </div>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="server-cert-pem">Certificate PEM</Label>
                        <Input
                          type="file"
                          accept=".pem,.crt,.cert,text/plain"
                          disabled={serverSettingsLoading || serverSettingsPending}
                          onChange={(event) => {
                            const input = event.currentTarget;
                            const file = input.files?.[0];
                            if (!file) return;
                            void readServerPEMUpload(file, setServerTLSCertPEM).finally(() => {
                              input.value = "";
                            });
                          }}
                        />
                        <Textarea
                          id="server-cert-pem"
                          value={serverTLSCertPEM}
                          onChange={(event) => setServerTLSCertPEM(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          placeholder="Paste certificate PEM"
                          className="h-32 min-h-32 max-h-48 resize-y overflow-auto [field-sizing:fixed] font-mono text-xs"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="server-key-pem">Private key PEM</Label>
                        <Input
                          type="file"
                          accept=".pem,.key,text/plain"
                          disabled={serverSettingsLoading || serverSettingsPending}
                          onChange={(event) => {
                            const input = event.currentTarget;
                            const file = input.files?.[0];
                            if (!file) return;
                            void readServerPEMUpload(file, setServerTLSKeyPEM).finally(() => {
                              input.value = "";
                            });
                          }}
                        />
                        <Textarea
                          id="server-key-pem"
                          value={serverTLSKeyPEM}
                          onChange={(event) => setServerTLSKeyPEM(event.target.value)}
                          disabled={serverSettingsLoading || serverSettingsPending}
                          placeholder="Paste private key PEM"
                          className="h-32 min-h-32 max-h-48 resize-y overflow-auto [field-sizing:fixed] font-mono text-xs"
                        />
                      </div>
                    </div>
                    {serverTLSEnabled && (!serverTLSCertAvailable || !serverTLSKeyAvailable) && (
                      <p className="text-xs text-destructive">
                        HTTPS requires a certificate and private key path or pasted PEM content.
                      </p>
                    )}
                    {(serverSettingsActionError || serverSettingsActionStatus) && (
                      <p className={cn("text-sm", serverSettingsActionError ? "text-destructive" : "text-muted-foreground")}>
                        {serverSettingsActionError ?? serverSettingsActionStatus}
                      </p>
                    )}
                    <div className="flex justify-end">
                      <Button
                        type="button"
                        onClick={saveServerSettings}
                        disabled={serverSettingsSaveDisabled}
                      >
                        {serverTLSCertPEM || serverTLSKeyPEM ? <Upload className="h-4 w-4" /> : <Save className="h-4 w-4" />}
                        Save Server
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-3 rounded-md border border-border p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-medium">Runtime updates</h3>
                        <p className="text-xs text-muted-foreground">
                          {toolUpdatesLoading ? "Loading versions" : runtimeUpdateSummary(toolUpdates)}
                        </p>
                      </div>
                      <label className="flex items-center gap-2 text-sm">
                        <Switch
                          checked={toolAutoEnabled}
                          disabled={toolUpdatesLoading || toolSettingsPending}
                          aria-label="Enable automatic runtime updates"
                          onCheckedChange={setToolAutoEnabled}
                        />
                        Auto
                      </label>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-[8rem_1fr]">
                      <div className="space-y-2">
                        <Label htmlFor="tool-update-time">Daily time</Label>
                        <Input
                          id="tool-update-time"
                          type="time"
                          value={toolTimeOfDay}
                          onChange={(event) => setToolTimeOfDay(event.target.value)}
                          disabled={toolUpdatesLoading || toolSettingsPending}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="tool-update-timezone">Timezone</Label>
                        <Input
                          id="tool-update-timezone"
                          value={toolTimezone}
                          onChange={(event) => setToolTimezone(event.target.value)}
                          disabled={toolUpdatesLoading || toolSettingsPending}
                          placeholder="Local or Asia/Shanghai"
                        />
                      </div>
                    </div>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm">
                        <Checkbox
                          checked={toolClaudeEnabled}
                          onChange={(event) => setToolClaudeEnabled(event.currentTarget.checked)}
                          disabled={toolUpdatesLoading || toolSettingsPending}
                          aria-label="Update Claude Code"
                        />
                        Claude Code
                      </label>
                      <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm">
                        <Checkbox
                          checked={toolCodexEnabled}
                          onChange={(event) => setToolCodexEnabled(event.currentTarget.checked)}
                          disabled={toolUpdatesLoading || toolSettingsPending}
                          aria-label="Update Codex"
                        />
                        Codex
                      </label>
                    </div>
                    {toolUpdates?.tools.map((tool) => (
                      <div key={tool.tool} className="grid gap-1 rounded-md bg-muted px-3 py-2 text-xs">
                        <div className="flex items-center justify-between gap-3">
                          <span className="font-medium">{tool.display_name}</span>
                          <span className="text-muted-foreground">{tool.state}</span>
                        </div>
                        <p className="truncate text-muted-foreground">
                          {tool.current_version || "unknown"}{tool.latest_version ? ` -> ${tool.latest_version}` : ""}
                          {tool.runtime_reset_pending ? " · restart pending" : ""}
                        </p>
                      </div>
                    ))}
                    {(toolActionError || toolActionStatus) && (
                      <p className={cn("text-sm", toolActionError ? "text-destructive" : "text-muted-foreground")}>
                        {toolActionError ?? toolActionStatus}
                      </p>
                    )}
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button type="button" variant="outline" onClick={checkAllToolUpdates} disabled={toolUpdatesLoading || toolSettingsPending}>
                        <RefreshCw className="h-4 w-4" />
                        Check
                      </Button>
                      <Button type="button" variant="outline" onClick={runAllToolUpdates} disabled={toolUpdatesLoading || toolSettingsPending}>
                        <Download className="h-4 w-4" />
                        Update
                      </Button>
                      <Button type="button" onClick={saveToolUpdateSettings} disabled={toolUpdatesLoading || toolSettingsPending}>
                        <Save className="h-4 w-4" />
                        Save Updates
                      </Button>
                    </div>
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setAccountSettingsOpen(false)}>
                    Close
                  </Button>
                  <Button variant="destructive" onClick={onLogout}>
                    <LogOut className="h-4 w-4" />
                    Log out
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>


  );
}

function runtimeUpdateSummary(overview?: ToolUpdateOverview): string {
  if (!overview) return "Version status unavailable";
  const updates = overview.tools.filter((tool) => tool.update_available).length;
  if (updates > 0) return `${updates} update${updates === 1 ? "" : "s"} available`;
  return overview.settings.auto_enabled
    ? `Auto updates at ${overview.settings.time_of_day} ${overview.settings.timezone}`
    : "Automatic updates disabled";
}
