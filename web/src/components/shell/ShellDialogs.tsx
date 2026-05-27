import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { Bot, Trash2 } from "lucide-react";
import type { Channel, NotificationSettings, Organization, Project, ServerSettings, ToolUpdateOverview, User, UserPreferences, Workspace } from "@/api/types";
import { cn } from "@/lib/utils";
import { AVATAR_COLORS, agentKindColor } from "../AgentAvatar";
import { agentKindFromProviderPersistent, agentPersistentFromKind, agentProviderFromKind } from "./utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { BrowserNotificationPermission } from "@/notifications/browser";
import { AccountSettingsDialog } from "./AccountSettingsDialog";
import { AGENT_EFFORT_OPTIONS, AGENT_RUNTIME_OPTIONS, initials } from "./utils";

type StringSetter = Dispatch<SetStateAction<string>>;
type BooleanSetter = Dispatch<SetStateAction<boolean>>;

export interface ShellDialogsProps {
  user: User;
  organization?: Organization;
  project?: Project;
  projectWorkspace?: Workspace;
  selectedChannel?: Channel;
  notificationSettings?: NotificationSettings;
  notificationSettingsLoading: boolean;
  serverSettings?: ServerSettings;
  serverSettingsLoading: boolean;
  serverSettingsError: string | null;
  toolUpdates?: ToolUpdateOverview;
  toolUpdatesLoading: boolean;
  preferences: UserPreferences;
  preferencesLoading: boolean;
  onLogout: () => void;

  threadEditOpen: boolean;
  setThreadEditOpen: BooleanSetter;
  threadTitleDraft: string;
  setThreadTitleDraft: StringSetter;
  threadActionError: string | null;
  threadActionPending: boolean;
  submitActiveThreadTitle: () => void | Promise<void>;

  accountSettingsOpen: boolean;
  setAccountSettingsOpen: BooleanSetter;
  browserPermission: BrowserNotificationPermission;
  enableBrowserNotifications: () => void | Promise<void>;
  preferencesPending: boolean;
  preferencesError: string | null;
  showTTFT: boolean;
  showTPS: boolean;
  hideAvatars: boolean;
  saveUserPreferences: (next: UserPreferences) => Promise<void>;
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

  projectEditOpen: boolean;
  setProjectEditOpen: BooleanSetter;
  projectEditName: string;
  setProjectEditName: StringSetter;
  projectEditWorkspacePath: string;
  setProjectEditWorkspacePath: StringSetter;
  projectEditEmoji: string;
  setProjectEditEmoji: StringSetter;
  projectEditColor: string;
  setProjectEditColor: StringSetter;
  projectEditError: string | null;
  projectEditPending: boolean;
  submitProjectEdit: () => void | Promise<void>;
  deleteActiveProject: () => void | Promise<void>;

  projectDraftOpen: boolean;
  setProjectDraftOpen: BooleanSetter;
  projectName: string;
  setProjectName: StringSetter;
  projectWorkspacePath: string;
  setProjectWorkspacePath: StringSetter;
  projectCreateError: string | null;
  setProjectCreateError: Dispatch<SetStateAction<string | null>>;
  projectCreatePending: boolean;
  submitProject: () => void | Promise<void>;

  channelDraftOpen: boolean;
  setChannelDraftOpen: BooleanSetter;
  channelName: string;
  setChannelName: StringSetter;
  channelType: Channel["type"];
  setChannelType: Dispatch<SetStateAction<Channel["type"]>>;
  channelTeamMaxBatches: string;
  setChannelTeamMaxBatches: StringSetter;
  channelTeamMaxRuns: string;
  setChannelTeamMaxRuns: StringSetter;
  submitChannel: () => void | Promise<void>;

  agentDraftOpen: boolean;
  setAgentDraftOpen: BooleanSetter;
  newAgentName: string;
  setNewAgentName: StringSetter;
  newAgentDescription: string;
  setNewAgentDescription: StringSetter;
  newAgentHandle: string;
  setNewAgentHandle: StringSetter;
  newAgentKind: string;
  setNewAgentKind: StringSetter;
  newAgentModel: string;
  setNewAgentModel: StringSetter;
  newAgentEffort: string;
  setNewAgentEffort: StringSetter;
  newAgentFastMode: boolean;
  setNewAgentFastMode: BooleanSetter;
  newAgentYoloMode: boolean;
  setNewAgentYoloMode: BooleanSetter;
  newAgentEmoji: string;
  setNewAgentEmoji: StringSetter;
  newAgentColor: string;
  setNewAgentColor: StringSetter;
  newAgentError: string | null;
  setNewAgentError: Dispatch<SetStateAction<string | null>>;
  creatingAgent: boolean;
  submitAgent: () => void | Promise<void>;
}

export function ShellDialogs({
  user,
  organization,
  project,
  projectWorkspace,
  selectedChannel,
  notificationSettings,
  notificationSettingsLoading,
  serverSettings,
  serverSettingsLoading,
  serverSettingsError,
  toolUpdates,
  toolUpdatesLoading,
  preferences,
  preferencesLoading,
  onLogout,
  threadEditOpen,
  setThreadEditOpen,
  threadTitleDraft,
  setThreadTitleDraft,
  threadActionError,
  threadActionPending,
  submitActiveThreadTitle,
  accountSettingsOpen,
  setAccountSettingsOpen,
  browserPermission,
  enableBrowserNotifications,
  preferencesPending,
  preferencesError,
  showTTFT,
  showTPS,
  hideAvatars,
  saveUserPreferences,
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
  projectEditOpen,
  setProjectEditOpen,
  projectEditName,
  setProjectEditName,
  projectEditWorkspacePath,
  setProjectEditWorkspacePath,
  projectEditEmoji,
  setProjectEditEmoji,
  projectEditColor,
  setProjectEditColor,
  projectEditError,
  projectEditPending,
  submitProjectEdit,
  deleteActiveProject,
  projectDraftOpen,
  setProjectDraftOpen,
  projectName,
  setProjectName,
  projectWorkspacePath,
  setProjectWorkspacePath,
  projectCreateError,
  setProjectCreateError,
  projectCreatePending,
  submitProject,
  channelDraftOpen,
  setChannelDraftOpen,
  channelName,
  setChannelName,
  channelType,
  setChannelType,
  channelTeamMaxBatches,
  setChannelTeamMaxBatches,
  channelTeamMaxRuns,
  setChannelTeamMaxRuns,
  submitChannel,
  agentDraftOpen,
  setAgentDraftOpen,
  newAgentName,
  setNewAgentName,
  newAgentDescription,
  setNewAgentDescription,
  newAgentHandle,
  setNewAgentHandle,
  newAgentKind,
  setNewAgentKind,
  newAgentModel,
  setNewAgentModel,
  newAgentEffort,
  setNewAgentEffort,
  newAgentFastMode,
  setNewAgentFastMode,
  newAgentYoloMode,
  setNewAgentYoloMode,
  newAgentEmoji,
  setNewAgentEmoji,
  newAgentColor,
  setNewAgentColor,
  newAgentError,
  setNewAgentError,
  creatingAgent,
  submitAgent,
}: ShellDialogsProps) {
  const [newAgentPersistentDraft, setNewAgentPersistentDraft] = useState(agentPersistentFromKind(newAgentKind));

  useEffect(() => {
    if (agentDraftOpen) {
      setNewAgentPersistentDraft(agentPersistentFromKind(newAgentKind));
    }
  }, [agentDraftOpen]);

  useEffect(() => {
    if (agentProviderFromKind(newAgentKind) !== "fake") {
      setNewAgentPersistentDraft(agentPersistentFromKind(newAgentKind));
    }
  }, [newAgentKind]);

  return (
    <>
            {/* Edit Post Modal */}
            <Dialog open={threadEditOpen} onOpenChange={setThreadEditOpen}>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Edit post</DialogTitle>
                  <DialogDescription>Update the post title shown in the forum catalog.</DialogDescription>
                </DialogHeader>
                <div className="space-y-2 py-2">
                  <Label htmlFor="thread-title">Title</Label>
                  <Input
                    id="thread-title"
                    value={threadTitleDraft}
                    onChange={(e) => setThreadTitleDraft(e.target.value)}
                    aria-label="Post title"
                    onKeyDown={(e) => { if (e.key === "Enter") submitActiveThreadTitle(); }}
                    autoFocus
                  />
                  {threadActionError && <p className="text-sm text-destructive">{threadActionError}</p>}
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setThreadEditOpen(false)} disabled={threadActionPending}>Cancel</Button>
                  <Button onClick={submitActiveThreadTitle} disabled={!threadTitleDraft.trim() || threadActionPending}>Save</Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            <AccountSettingsDialog
              user={user}
              organization={organization}
              project={project}
              accountSettingsOpen={accountSettingsOpen}
              setAccountSettingsOpen={setAccountSettingsOpen}
              browserPermission={browserPermission}
              enableBrowserNotifications={enableBrowserNotifications}
              preferences={preferences}
              preferencesLoading={preferencesLoading}
              preferencesPending={preferencesPending}
              preferencesError={preferencesError}
              showTTFT={showTTFT}
              showTPS={showTPS}
              hideAvatars={hideAvatars}
              saveUserPreferences={saveUserPreferences}
              notificationSettings={notificationSettings}
              notificationSettingsLoading={notificationSettingsLoading}
              webhookEnabled={webhookEnabled}
              setWebhookEnabled={setWebhookEnabled}
              webhookURL={webhookURL}
              setWebhookURL={setWebhookURL}
              webhookSecret={webhookSecret}
              setWebhookSecret={setWebhookSecret}
              notificationActionError={notificationActionError}
              notificationActionStatus={notificationActionStatus}
              notificationTestPending={notificationTestPending}
              notificationSavePending={notificationSavePending}
              sendTestWebhook={sendTestWebhook}
              saveNotificationSettings={saveNotificationSettings}
              serverSettings={serverSettings}
              serverSettingsLoading={serverSettingsLoading}
              serverSettingsError={serverSettingsError}
              toolUpdates={toolUpdates}
              toolUpdatesLoading={toolUpdatesLoading}
              toolAutoEnabled={toolAutoEnabled}
              setToolAutoEnabled={setToolAutoEnabled}
              toolTimeOfDay={toolTimeOfDay}
              setToolTimeOfDay={setToolTimeOfDay}
              toolTimezone={toolTimezone}
              setToolTimezone={setToolTimezone}
              toolClaudeEnabled={toolClaudeEnabled}
              setToolClaudeEnabled={setToolClaudeEnabled}
              toolCodexEnabled={toolCodexEnabled}
              setToolCodexEnabled={setToolCodexEnabled}
              toolSettingsPending={toolSettingsPending}
              toolActionStatus={toolActionStatus}
              toolActionError={toolActionError}
              saveToolUpdateSettings={saveToolUpdateSettings}
              checkAllToolUpdates={checkAllToolUpdates}
              runAllToolUpdates={runAllToolUpdates}
              serverListenIP={serverListenIP}
              setServerListenIP={setServerListenIP}
              serverListenPort={serverListenPort}
              setServerListenPort={setServerListenPort}
              serverTLSEnabled={serverTLSEnabled}
              setServerTLSEnabled={setServerTLSEnabled}
              serverTLSListenPort={serverTLSListenPort}
              setServerTLSListenPort={setServerTLSListenPort}
              serverTLSCertFile={serverTLSCertFile}
              setServerTLSCertFile={setServerTLSCertFile}
              serverTLSKeyFile={serverTLSKeyFile}
              setServerTLSKeyFile={setServerTLSKeyFile}
              serverTLSCertPEM={serverTLSCertPEM}
              setServerTLSCertPEM={setServerTLSCertPEM}
              serverTLSKeyPEM={serverTLSKeyPEM}
              setServerTLSKeyPEM={setServerTLSKeyPEM}
              serverListenPortValid={serverListenPortValid}
              serverTLSListenPortValid={serverTLSListenPortValid}
              serverPortsConflict={serverPortsConflict}
              serverTLSCertAvailable={serverTLSCertAvailable}
              serverTLSKeyAvailable={serverTLSKeyAvailable}
              serverSettingsPending={serverSettingsPending}
              serverSettingsActionError={serverSettingsActionError}
              serverSettingsActionStatus={serverSettingsActionStatus}
              serverSettingsSaveDisabled={serverSettingsSaveDisabled}
              readServerPEMUpload={readServerPEMUpload}
              saveServerSettings={saveServerSettings}
              onLogout={onLogout}
            />

            {/* Edit Project Modal */}
            <Dialog open={projectEditOpen} onOpenChange={setProjectEditOpen}>
              <DialogContent onOpenAutoFocus={(event) => event.preventDefault()}>
                <DialogHeader>
                  <DialogTitle>Project settings</DialogTitle>
                  <DialogDescription>Update or delete this project.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-2">
                  <div className="flex items-center gap-4">
                    <div
                      className={cn(
                        "flex h-14 w-14 shrink-0 items-center justify-center rounded-xl text-white",
                        projectEditEmoji ? projectEditColor || "bg-primary" : "bg-primary"
                      )}
                    >
                      {projectEditEmoji ? (
                        <span className="text-2xl">{projectEditEmoji}</span>
                      ) : (
                        <span className="text-lg font-semibold">{initials(projectEditName || project?.name || "Project")}</span>
                      )}
                    </div>
                    <div className="min-w-0 flex-1 space-y-2">
                      <Input
                        value={projectEditEmoji}
                        onChange={(e) => setProjectEditEmoji(e.target.value)}
                        placeholder="Icon emoji"
                        aria-label="Project icon"
                      />
                      <div className="flex flex-wrap gap-1.5">
                        {AVATAR_COLORS.map((color) => (
                          <button
                            key={color}
                            className={cn(
                              "h-5 w-5 rounded-full transition-all",
                              color,
                              projectEditColor === color
                                ? "ring-2 ring-ring ring-offset-1 ring-offset-background"
                                : "opacity-60 hover:opacity-100"
                            )}
                            aria-label="Project color"
                            onClick={() => setProjectEditColor(color)}
                            type="button"
                          />
                        ))}
                        {projectEditEmoji && (
                          <button
                            className="h-5 rounded-full border border-border px-2 text-[10px] text-muted-foreground hover:text-foreground"
                            onClick={() => {
                              setProjectEditEmoji("");
                              setProjectEditColor("");
                            }}
                            type="button"
                          >
                            Reset
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="project-edit-name">Project name</Label>
                    <Input
                      id="project-edit-name"
                      value={projectEditName}
                      onChange={(e) => setProjectEditName(e.target.value)}
                      aria-label="Project name"
                      onKeyDown={(e) => { if (e.key === "Enter") void submitProjectEdit(); }}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="project-edit-workspace">Workspace path</Label>
                    <Input
                      id="project-edit-workspace"
                      value={projectEditWorkspacePath}
                      onChange={(e) => setProjectEditWorkspacePath(e.target.value)}
                      placeholder={projectWorkspace ? "" : "Loading workspace..."}
                      aria-label="Workspace path"
                      onKeyDown={(e) => { if (e.key === "Enter") void submitProjectEdit(); }}
                    />
                  </div>
                  {projectEditError && <p className="text-sm text-destructive">{projectEditError}</p>}
                </div>
                <DialogFooter className="sm:justify-between">
                  <Button
                    variant="destructive"
                    onClick={deleteActiveProject}
                    disabled={!project || projectEditPending}
                  >
                    <Trash2 className="h-4 w-4" />
                    Delete
                  </Button>
                  <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                  <Button
                    variant="outline"
                    onClick={() => setProjectEditOpen(false)}
                    disabled={projectEditPending}
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={submitProjectEdit}
                    disabled={!projectEditName.trim() || !projectEditWorkspacePath.trim() || projectEditPending}
                  >
                    Save
                  </Button>
                  </div>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* Create Project Modal */}
            <Dialog
              open={projectDraftOpen}
              onOpenChange={(open) => {
                setProjectDraftOpen(open);
                if (!open) setProjectCreateError(null);
              }}
            >
              <DialogContent onOpenAutoFocus={(event) => event.preventDefault()}>
                <DialogHeader>
                  <DialogTitle>Create project</DialogTitle>
                  <DialogDescription>Add a new project to organize your channels and agents.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-2">
                  <div className="space-y-2">
                    <Label htmlFor="project-name">Project name</Label>
                    <Input
                      id="project-name"
                      value={projectName}
                      onChange={(e) => setProjectName(e.target.value)}
                      placeholder="My Project"
                      aria-label="Project name"
                      onKeyDown={(e) => { if (e.key === "Enter") void submitProject(); }}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="project-workspace">Workspace path</Label>
                    <Input
                      id="project-workspace"
                      value={projectWorkspacePath}
                      onChange={(e) => setProjectWorkspacePath(e.target.value)}
                      placeholder="Default workspace"
                      aria-label="Workspace path"
                      onKeyDown={(e) => { if (e.key === "Enter") void submitProject(); }}
                    />
                  </div>
                  {projectCreateError && <p className="text-sm text-destructive">{projectCreateError}</p>}
                </div>
                <DialogFooter>
                  <Button
                    variant="outline"
                    onClick={() => setProjectDraftOpen(false)}
                    disabled={projectCreatePending}
                  >
                    Cancel
                  </Button>
                  <Button onClick={submitProject} disabled={!projectName.trim() || projectCreatePending}>
                    Save
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* Create Channel Modal */}
            <Dialog open={channelDraftOpen} onOpenChange={setChannelDraftOpen}>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create channel</DialogTitle>
                  <DialogDescription>Add a new channel to this project.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-2">
                  <div className="space-y-2">
                    <Label htmlFor="channel-name">Channel name</Label>
                    <Input
                      id="channel-name"
                      value={channelName}
                      onChange={(e) => setChannelName(e.target.value)}
                      placeholder="general"
                      aria-label="Channel name"
                      onKeyDown={(e) => { if (e.key === "Enter") submitChannel(); }}
                      autoFocus
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="channel-type">Channel type</Label>
                    <Select
                      id="channel-type"
                      value={channelType}
                      onChange={(e) => setChannelType(e.target.value as Channel["type"])}
                      aria-label="Channel type"
                    >
                      <option value="text">Text</option>
                      <option value="thread">Forum</option>
                    </Select>
                  </div>
                  <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
                    <div className="space-y-1">
                      <p className="text-sm font-medium">Team discussion budget</p>
                      <p className="text-xs leading-5 text-muted-foreground">
                        Used by /discuss. The first selected agent leads each round, and agent runs cap total sequential replies before the final answer.
                      </p>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-2">
                        <Label htmlFor="channel-team-batches">Discussion rounds</Label>
                        <Input
                          id="channel-team-batches"
                          type="number"
                          min={1}
                          max={20}
                          value={channelTeamMaxBatches}
                          onChange={(e) => setChannelTeamMaxBatches(e.target.value)}
                          aria-label="Team discussion rounds"
                          title="Maximum leader-led discussion rounds before the final answer."
                        />
                        <p className="text-[11px] leading-4 text-muted-foreground">1-20 handoff rounds.</p>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="channel-team-runs">Agent run budget</Label>
                        <Input
                          id="channel-team-runs"
                          type="number"
                          min={1}
                          max={50}
                          value={channelTeamMaxRuns}
                          onChange={(e) => setChannelTeamMaxRuns(e.target.value)}
                          aria-label="Team agent run budget"
                          title="Maximum sequential agent replies across the leader-led discussion."
                        />
                        <p className="text-[11px] leading-4 text-muted-foreground">1-50 discussion replies.</p>
                      </div>
                    </div>
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setChannelDraftOpen(false)}>Cancel</Button>
                  <Button
                    onClick={submitChannel}
                    disabled={!channelName.trim() || !channelTeamMaxBatches.trim() || !channelTeamMaxRuns.trim()}
                  >
                    Create
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* Create Agent Modal */}
            <Dialog
              open={agentDraftOpen}
              onOpenChange={(open) => {
                setAgentDraftOpen(open);
                if (open) setNewAgentError(null);
              }}
            >
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create agent</DialogTitle>
                  <DialogDescription>Add a new agent to this organization.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-2">
                  {/* Avatar */}
                  <div className="flex items-center gap-4">
                    <div className={cn(
                      "flex h-14 w-14 items-center justify-center rounded-full shrink-0",
                      newAgentEmoji
                        ? (newAgentColor || agentKindColor(newAgentKind))
                        : agentKindColor(newAgentKind)
                    )}>
                      {newAgentEmoji ? (
                        <span className="text-2xl">{newAgentEmoji}</span>
                      ) : (
                        <Bot className="h-7 w-7 text-white" />
                      )}
                    </div>
                    <div className="flex-1 space-y-2">
                      <Input
                        value={newAgentEmoji}
                        onChange={(e) => setNewAgentEmoji(e.target.value)}
                        placeholder="Avatar emoji (e.g. 🤖)"
                        aria-label="New agent avatar"
                      />
                      <div className="flex gap-1.5 flex-wrap">
                        {AVATAR_COLORS.map((c) => (
                          <button
                            key={c}
                            className={cn(
                              "h-5 w-5 rounded-full transition-all",
                              c,
                              newAgentColor === c ? "ring-2 ring-ring ring-offset-1 ring-offset-background" : "opacity-60 hover:opacity-100"
                            )}
                            onClick={() => setNewAgentColor(c)}
                            type="button"
                          />
                        ))}
                      </div>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="new-agent-name">Name</Label>
                    <Input
                      id="new-agent-name"
                      value={newAgentName}
                      onChange={(e) => {
                        setNewAgentName(e.target.value);
                        setNewAgentError(null);
                      }}
                      placeholder="My Agent"
                      aria-label="New agent name"
                      autoFocus
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="new-agent-description">Description</Label>
                    <Textarea
                      id="new-agent-description"
                      value={newAgentDescription}
                      onChange={(e) => setNewAgentDescription(e.target.value)}
                      placeholder="What this agent is responsible for"
                      aria-label="New agent description"
                      rows={3}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="new-agent-handle">Handle</Label>
                    <Input
                      id="new-agent-handle"
                      value={newAgentHandle}
                      onChange={(e) => {
                        setNewAgentHandle(e.target.value);
                        setNewAgentError(null);
                      }}
                      placeholder="my_agent"
                      aria-label="New agent handle"
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="new-agent-runtime">Runtime</Label>
                      <Select
                        id="new-agent-runtime"
                        value={agentProviderFromKind(newAgentKind)}
                        onChange={(e) =>
                          setNewAgentKind(agentKindFromProviderPersistent(e.target.value, newAgentPersistentDraft))
                        }
                        aria-label="New agent runtime"
                      >
                        {AGENT_RUNTIME_OPTIONS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="new-agent-model">Model</Label>
                      <Input
                        id="new-agent-model"
                        value={newAgentModel}
                        onChange={(e) => setNewAgentModel(e.target.value)}
                        placeholder="default"
                        aria-label="New agent model"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="new-agent-effort">Effort</Label>
                      <Input
                        id="new-agent-effort"
                        value={newAgentEffort}
                        onChange={(e) => setNewAgentEffort(e.target.value)}
                        list="new-agent-effort-suggestions"
                        placeholder="default or custom"
                        aria-label="New agent effort"
                        autoComplete="off"
                      />
                      <datalist id="new-agent-effort-suggestions">
                        {AGENT_EFFORT_OPTIONS.map((option) => (
                          <option key={option} value={option} />
                        ))}
                      </datalist>
                    </div>
                  </div>
                  {agentProviderFromKind(newAgentKind) !== "fake" && (
                    <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                      <Checkbox
                        checked={newAgentPersistentDraft}
                        onChange={(e) => {
                          setNewAgentPersistentDraft(e.target.checked);
                          setNewAgentKind(agentKindFromProviderPersistent(agentProviderFromKind(newAgentKind), e.target.checked));
                        }}
                        aria-label="New agent persistent process"
                      />
                      Persistent process
                    </label>
                  )}
                  <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                    <Checkbox
                      checked={newAgentFastMode}
                      onChange={(e) => setNewAgentFastMode(e.target.checked)}
                      aria-label="New agent fast mode"
                    />
                    Fast mode
                  </label>
                  <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                    <Checkbox
                      checked={newAgentYoloMode}
                      onChange={(e) => setNewAgentYoloMode(e.target.checked)}
                      aria-label="New agent YOLO mode"
                    />
                    YOLO mode
                  </label>
                  {newAgentError && (
                    <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                      {newAgentError}
                    </p>
                  )}
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setAgentDraftOpen(false)} disabled={creatingAgent}>Cancel</Button>
                  <Button onClick={submitAgent} disabled={!newAgentName.trim() || creatingAgent}>Create</Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

    </>
  );
}
