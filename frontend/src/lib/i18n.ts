import type { Lang } from './domain'

/**
 * EN/ID dictionaries (ARCHITECTURE 18.2: i18n is a Promise resource unwrapped
 * with React 19's `use()`).
 *
 * Both dictionaries are complete: `Dictionary` is derived from the English keys,
 * so adding a key to EN without adding it to ID is a compile error, and a
 * missing translation can never silently fall back to an English string in the
 * middle of an Indonesian page. EN is the default and the fallback.
 */
export interface Dictionary {
  'nav.boards': string
  'nav.projects': string
  'nav.tasks': string
  'nav.agents': string
  'nav.finops': string
  'nav.approvals': string
  'nav.settings': string
  'action.newProject': string
  'action.create': string
  'action.cancel': string
  'action.retry': string
  'action.open': string
  'field.name': string
  'field.slug': string
  'field.title': string
  'field.description': string
  'field.priority': string
  'field.email': string
  'field.password': string
  'field.newPassword': string
  'field.confirmPassword': string
  'empty.projects': string
  'empty.boards': string
  'empty.tasks': string
  'state.loading': string
  'state.error': string
  'cost.today': string
  'cost.runningNow': string
  // Auth screens (01-login, 02-register, 03-reset-request, 04-reset-confirm).
  // The Indonesian strings are the PRD's own copy — US-AD02 AC5 names the
  // button "Log in" and the link "Lupa password?" — so ID is the default
  // locale and EN is the alternate, not the other way round.
  'auth.tagline': string
  'auth.login.title': string
  'auth.login.subtitle': string
  'auth.login.submit': string
  'auth.login.pending': string
  'auth.login.forgot': string
  'auth.register.title': string
  'auth.register.subtitle': string
  'auth.register.submit': string
  'auth.register.pending': string
  'auth.register.passwordHint': string
  'auth.register.workspaceBadgeTitle': string
  'auth.register.workspaceBadgeText': string
  'auth.resetRequest.title': string
  'auth.resetRequest.subtitle': string
  'auth.resetRequest.submit': string
  'auth.resetRequest.pending': string
  'auth.resetRequest.sent': string
  'auth.resetConfirm.title': string
  'auth.resetConfirm.subtitle': string
  'auth.resetConfirm.submit': string
  'auth.resetConfirm.pending': string
  'auth.resetConfirm.mismatch': string
  'auth.resetConfirm.success': string
  'auth.securityNoticeTitle': string
  'auth.resetRequest.notice': string
  'auth.resetConfirm.notice': string
  'auth.haveAccount': string
  'auth.noAccount': string
  'auth.signIn': string
  /** Sign-out action in the account menu (US-AD89 AC5). */
  'auth.logout': string
  'auth.rememberedPassword': string

  // Settings → Members (US-AD04). The roster.
  'members.title': string
  'members.subtitle': string
  'members.joined': string
  'members.search': string
  'members.count': string
  'members.you': string
  'members.ownerLocked': string
  'members.empty': string
  'members.emptyHint': string
  'members.colUser': string
  'members.colEmail': string
  'members.colRole': string
  'members.colJoined': string
  'members.colAction': string
  'members.changeRole': string
  'members.remove': string
  // Invite (US-AD04 AC1).
  'members.invite.cta': string
  'members.invite.title': string
  'members.invite.description': string
  'members.invite.submit': string
  'members.invite.pending': string
  'members.invite.done': string
  'members.invite.failed': string
  // Change role dialog.
  'members.role.title': string
  'members.role.description': string
  'members.role.submit': string
  'members.role.failed': string
  'members.role.ownerHint': string
  // Remove dialog.
  'members.remove.title': string
  'members.remove.description': string
  'members.remove.submit': string
  'members.remove.failed': string
  // Roles, DECISIONS 4.
  'role.owner': string
  'role.admin': string
  'role.member': string
  'role.viewer': string
  // Shown when the roster could not be read.
  'members.loadFailed': string
  // Workspace settings (US-AD77).
  'workspace.title': string
  'workspace.subtitle': string
  'workspace.badge': string
  'workspace.description': string
  'workspace.soloTitle': string
  'workspace.soloText': string
  'workspace.soloStatus': string
  'workspace.cleanTitle': string
  'workspace.cleanText': string
  'workspace.cleanStatus': string
  'workspace.switcher': string
  'workspace.summaryTitle': string
  'workspace.summaryHint': string
  'workspace.id': string
  'workspace.displayName': string
  'workspace.slug': string
  'workspace.kind': string
  'workspace.role': string
  'workspace.editName': string
  'workspace.editTitle': string
  'workspace.editDescription': string
  'workspace.save': string
  'workspace.savePending': string
  'workspace.resourceTitle': string
  'workspace.resourceUnavailable': string
  'workspace.securityTitle': string
  'workspace.securityText': string
  'workspace.updateFailed': string
  'workspace.updated': string
  // Profil akun sendiri (15-profile, US-AD89). `profile.avatarAuto` is the
  // design's "Otomatis" chip: the avatar is derived server-side from the name,
  // so the field is read-only and says so rather than offering an upload the
  // API does not implement yet.
  'profile.title': string
  'profile.subtitle': string
  'profile.description': string
  'profile.menu': string
  'profile.menuOpen': string
  'profile.account': string
  'profile.accountActive': string
  'profile.idField': string
  'profile.idHint': string
  'profile.emailField': string
  'profile.emailHint': string
  'profile.nameField': string
  'profile.nameHint': string
  'profile.avatarField': string
  'profile.avatarAuto': string
  'profile.avatarHint': string
  'profile.save': string
  'profile.savePending': string
  'profile.reset': string
  'profile.saved': string
  'profile.saveFailed': string
  'profile.workspacesTitle': string
  'profile.workspacesHint': string
  'profile.workspaceRole': string
  'profile.workspaceKind': string
  'profile.empty': string
  'profile.roleNote': string
  'profile.readOnly': string
  'profile.editable': string

  // Boards (US-AD09, US-AD91 AC3).
  'boards.new': string
  'boards.create.title': string
  'boards.create.description': string
  'boards.create.project': string
  'boards.create.columnsHint': string
  'boards.create.submit': string
  'boards.create.pending': string
  'boards.create.failed': string
  'boards.empty': string
  'boards.emptyHint': string
  'boards.createFirst': string
  'boards.noProjects': string
  'boards.noProjectsHint': string
  // Agent registry (25-agent-registry, 26-agent-form) — US-AD20.
  'agents.title': string
  'agents.new': string
  'agents.search': string
  'agents.col.agent': string
  'agents.col.provider': string
  'agents.col.model': string
  'agents.col.reasoning': string
  'agents.col.status': string
  'agents.col.runtime': string
  'agents.col.tools': string
  'agents.col.actions': string
  'agents.count': string
  'agents.empty': string
  'agents.emptyHint': string
  'agents.noProjects': string
  'agents.noProjectsHint': string
  'agents.create.title': string
  'agents.create.description': string
  'agents.create.submit': string
  'agents.create.pending': string
  'agents.create.failed': string
  'agents.field.provider': string
  'agents.field.model': string
  'agents.field.modelHint': string
  'agents.field.reasoning': string
  'agents.field.runtime': string
  'agents.field.retry': string
  'agents.field.attempts': string
  'agents.field.tools': string
  'agents.field.toolsHint': string
  'agents.field.skills': string
  'agents.field.skillsHint': string
  'agents.delete': string
  'agents.delete.title': string
  'agents.delete.body': string
  'agents.delete.confirm': string
  'agents.delete.pending': string
  'agents.delete.failed': string
  'agents.status.ready': string
  'agents.status.needsKey': string
  'agents.status.archived': string
  // Toolbar, table footer and the two guidance cards of 25-agent-registry.
  'agents.filter.all': string
  'agents.filter.label': string
  'agents.filter.empty': string
  'agents.filter.emptyHint': string
  'agents.footer.showing': string
  'agents.footer.ready': string
  'agents.footer.archived': string
  'agents.statusCard.title': string
  'agents.statusCard.hint': string
  'agents.statusCard.ready': string
  'agents.statusCard.needsKey': string
  'agents.statusCard.archived': string
  'agents.spec.title': string
  'agents.spec.badge': string
  'agents.spec.intro': string
  'agents.spec.credentialLabel': string
  'agents.spec.credentialBody': string
  'agents.archive.title': string
  'agents.archive.badge': string
  'agents.archive.runningLabel': string
  'agents.archive.runningBody': string
  'agents.archive.assignLabel': string
  'agents.archive.assignBody': string
  'agents.archive.assignable': string
  'agents.archive.hidden': string
  'agents.archive.note': string
  'agents.detail.back': string
  'agents.detail.title': string
  'agents.detail.statusActive': string
  'agents.detail.statusArchived': string
  'agents.detail.statusNeedsKey': string
  'agents.detail.id': string
  'agents.detail.created': string
  'agents.detail.runs': string
  'agents.detail.runsHint': string
  'agents.detail.cost': string
  'agents.detail.costEstimate': string
  'agents.detail.providerTitle': string
  'agents.detail.providerVerified': string
  'agents.detail.provider': string
  'agents.detail.model': string
  'agents.detail.modelHint': string
  'agents.detail.modelUnavailable': string
  'agents.detail.baseUrl': string
  'agents.detail.baseUrlHint': string
  'agents.detail.runtimeTitle': string
  'agents.detail.runtimeSubtitle': string
  'agents.detail.runtime': string
  'agents.detail.retry': string
  'agents.detail.reasoning': string
  'agents.detail.attempts': string
  'agents.detail.tools': string
  'agents.detail.skills': string
  'agents.detail.tasksTitle': string
  'agents.detail.tasksEmpty': string
  'agents.detail.save': string
  'agents.detail.saving': string
  'agents.detail.saved': string
  'agents.detail.saveFailed': string
  'agents.detail.loading': string
  'agents.detail.notFound': string
  'agents.detail.archiveAction': string
  'agents.detail.unarchiveAction': string
  'agents.detail.archiveHint': string
  'agents.archive.forbidden': string
  'agents.archive.running': string
  'agents.archive.failed': string
}

const en: Dictionary = {
  'nav.boards': 'Boards',
  'nav.projects': 'Projects',
  'nav.tasks': 'Tasks',
  'nav.agents': 'Agents',
  'nav.finops': 'Cost & Usage',
  'nav.approvals': 'Approvals',
  'nav.settings': 'Settings',
  'action.newProject': 'New project',
  'action.create': 'Create',
  'action.cancel': 'Cancel',
  'action.retry': 'Retry',
  'action.open': 'Open',
  'field.name': 'Name',
  'field.slug': 'Slug',
  'field.title': 'Title',
  'field.description': 'Description',
  'field.priority': 'Priority',
  'field.email': 'Email',
  'field.password': 'Password',
  'field.newPassword': 'New password',
  'field.confirmPassword': 'Confirm new password',
  'empty.projects': 'No projects yet',
  'empty.boards': 'No boards yet',
  'empty.tasks': 'No tasks yet',
  'state.loading': 'Loading…',
  'state.error': 'Something went wrong',
  'cost.today': 'Today',
  'cost.runningNow': 'Running now',
  'auth.tagline': 'Fleet Orchestration',
  'auth.login.title': 'Sign in to AgentDeck',
  'auth.login.subtitle': 'Enter your account credentials to reach the fleet.',
  'auth.login.submit': 'Log in',
  'auth.login.pending': 'Signing in…',
  'auth.login.forgot': 'Forgot password?',
  'auth.register.title': 'Create an AgentDeck account',
  'auth.register.subtitle': 'Start orchestrating your agent fleet with a new account.',
  'auth.register.submit': 'Create account',
  'auth.register.pending': 'Creating…',
  'auth.register.passwordHint': 'min. 8 characters',
  'auth.register.workspaceBadgeTitle': 'Personal workspace created automatically',
  'auth.register.workspaceBadgeText':
    'A personal workspace is created with no organisation setup. You can go straight to your first project and board.',
  'auth.resetRequest.title': 'Forgot password',
  'auth.resetRequest.subtitle': 'Enter your account email. We will send a verification link to set a new password.',
  'auth.resetRequest.submit': 'Send reset link',
  'auth.resetRequest.pending': 'Sending…',
  'auth.resetRequest.sent': 'If that email is registered, a reset link is on its way.',
  'auth.resetConfirm.title': 'Create a new password',
  'auth.resetConfirm.subtitle':
    'Enter a new password for your account. Make sure it meets the fleet security standard.',
  'auth.resetConfirm.submit': 'Save new password',
  'auth.resetConfirm.pending': 'Saving…',
  'auth.resetConfirm.mismatch': 'The two passwords do not match.',
  'auth.resetConfirm.success': 'Password updated. Every other session was signed out.',
  'auth.securityNoticeTitle': 'Security',
  'auth.resetRequest.notice':
    'A reset link requires a new password of at least 8 characters. After a successful reset, every login session ({sessions}) on other devices is revoked automatically.',
  'auth.resetConfirm.notice':
    'The new password must be at least 8 characters. After a successful reset the {users.password_hash} column is updated and every {sessions} row for the account is revoked automatically.',
  'auth.haveAccount': 'Already have an account?',
  'auth.noAccount': 'New here?',
  'auth.signIn': 'Sign in',
  'auth.logout': 'Sign out',
  'auth.rememberedPassword': 'Remembered your password?',

  // Settings → Members (US-AD04).
  'members.title': 'Members & roles',
  'members.subtitle': 'Workspace roster',
  'members.joined': 'Joined',
  'members.search': 'Search name, email, role…',
  'members.count': '{0} members',
  'members.you': 'you',
  'members.ownerLocked': 'Full owner',
  'members.empty': 'No members yet',
  'members.emptyHint': 'Invite someone by email to give them access to this workspace.',
  'members.colUser': 'User',
  'members.colEmail': 'Email',
  'members.colRole': 'Role',
  'members.colJoined': 'Joined',
  'members.colAction': 'Action',
  'members.changeRole': 'Change role',
  'members.remove': 'Remove',
  'members.invite.cta': 'Invite member',
  'members.invite.title': 'Invite member',
  'members.invite.description': 'They are added with the role you pick. The address is emailed a notice.',
  'members.invite.submit': 'Send invite',
  'members.invite.pending': 'Sending…',
  'members.invite.done': 'Invite sent to {0} as {1}.',
  'members.invite.failed': 'The invite was not sent',
  'members.role.title': 'Change role',
  'members.role.description': 'The new role applies immediately.',
  'members.role.submit': 'Save role',
  'members.role.failed': 'The role was not changed',
  'members.role.ownerHint': 'Owner is granted when a workspace is created and cannot be assigned here.',
  'members.remove.title': 'Remove member',
  'members.remove.description': 'They lose access to this workspace. Their account is not deleted.',
  'members.remove.submit': 'Remove',
  'members.remove.failed': 'The member was not removed',
  'role.owner': 'Owner',
  'role.admin': 'Admin',
  'role.member': 'Member',
  'role.viewer': 'Viewer',
  'members.loadFailed': 'The roster could not be loaded',
  'workspace.title': 'Workspace settings',
  'workspace.subtitle': 'Organization settings',
  'workspace.badge': 'Settings',
  'workspace.description': 'A summary of the active workspace. Core work can be completed without opening this page.',
  'workspace.soloTitle': 'Solo flow stays unblocked',
  'workspace.soloText':
    'A user with one workspace can create projects, register agents, and run boards without visiting settings.',
  'workspace.soloStatus': 'Core B2C flow is active',
  'workspace.cleanTitle': 'No redundant workspace selector',
  'workspace.cleanText':
    'When there is only one workspace, the top bar stays clean and does not render a one-item dropdown.',
  'workspace.cleanStatus': 'Top bar has no redundant selector',
  'workspace.switcher': 'Switch workspace',
  'workspace.summaryTitle': 'Active workspace summary',
  'workspace.summaryHint':
    'Real values from the active workspace; resource totals are not available in this milestone.',
  'workspace.id': 'Workspace ID',
  'workspace.displayName': 'Display name',
  'workspace.slug': 'Slug',
  'workspace.kind': 'Ownership type',
  'workspace.role': 'Your role',
  'workspace.editName': 'Rename workspace',
  'workspace.editTitle': 'Rename workspace',
  'workspace.editDescription': 'The slug stays unchanged. This change is recorded in the workspace audit trail.',
  'workspace.save': 'Save name',
  'workspace.savePending': 'Saving…',
  'workspace.resourceTitle': 'Registered resources',
  'workspace.resourceUnavailable':
    'Resource totals and cost summaries arrive with their respective M1/M2 endpoints. No placeholder numbers are shown here.',
  'workspace.securityTitle': 'Org-scoped data isolation',
  'workspace.securityText':
    'Projects, boards, members, approvals, and cost data are requested in the active workspace context.',
  'workspace.updateFailed': 'The workspace name was not changed',
  'workspace.updated': 'Workspace name updated',
  'profile.title': 'Own account profile',
  'profile.subtitle': 'Identity and workspace memberships',
  'profile.description':
    'Your identity is isolated per login session and read straight from the daemon endpoint GET /api/v1/auth/me.',
  'profile.menu': 'Open profile menu',
  'profile.menuOpen': 'Profile menu',
  'profile.account': 'Active account:',
  'profile.accountActive': 'Signed in as {0}',
  'profile.idField': 'id (user id)',
  'profile.idHint': 'Internal daemon identifier',
  'profile.emailField': 'email',
  'profile.emailHint': 'The address used to sign in',
  'profile.nameField': 'name',
  'profile.nameHint': 'Display name in the top bar and drawers',
  'profile.avatarField': 'avatar_user',
  'profile.avatarAuto': 'Automatic',
  'profile.avatarHint': 'Derived from the display name by the server',
  'profile.save': 'Save changes',
  'profile.savePending': 'Saving…',
  'profile.reset': 'Reset',
  'profile.saved': 'Profile updated',
  'profile.saveFailed': 'The profile was not changed',
  'profile.workspacesTitle': 'Workspace memberships',
  'profile.workspacesHint': 'Every workspace this account belongs to, with the role it holds there.',
  'profile.workspaceRole': 'Role',
  'profile.workspaceKind': 'Ownership',
  'profile.empty': 'No workspace membership is recorded for this account.',
  'profile.roleNote': 'Opening this page needs no owner or admin role.',
  'profile.readOnly': 'Read-only',
  'profile.editable': 'Editable',

  'boards.new': 'New board',
  'boards.create.title': 'New board',
  'boards.create.description': 'The board is created inside the project you pick, in the active workspace.',
  'boards.create.project': 'Project',
  'boards.create.columnsHint': 'Starts with the default columns: Backlog, Ready, Running, Review, Done.',
  'boards.create.submit': 'Create',
  'boards.create.pending': 'Creating…',
  'boards.create.failed': 'The board was not created',
  'boards.empty': 'No boards yet',
  'boards.emptyHint': 'Create the first board in this workspace to start moving work.',
  'boards.createFirst': 'Create your first board',
  'boards.noProjects': 'No projects yet',
  'boards.noProjectsHint': 'A board lives inside a project. Create one on the Projects page first.',
  'agents.title': 'Agent Registry',
  'agents.new': 'Register agent',
  'agents.search': 'Search agent, model, or skill...',
  'agents.col.agent': 'Agent / ID',
  'agents.col.provider': 'Provider',
  'agents.col.model': 'Model',
  'agents.col.reasoning': 'Reasoning',
  'agents.col.status': 'Fleet status',
  'agents.col.runtime': 'Runtime & retry',
  'agents.col.tools': 'Tools / skills',
  'agents.col.actions': 'Actions',
  'agents.count': 'agents registered',
  'agents.empty': 'No agents in this project',
  'agents.emptyHint': 'Register one to give tasks a runner.',
  'agents.noProjects': 'No projects yet',
  'agents.noProjectsHint': 'An agent belongs to a project. Create one on the Projects page first.',
  'agents.create.title': 'Register agent',
  'agents.create.description': 'The agent becomes the retry and runtime limit source for every task it runs.',
  'agents.create.submit': 'Save',
  'agents.create.pending': 'Saving…',
  'agents.create.failed': 'The agent was not registered',
  'agents.field.provider': 'Provider',
  'agents.field.model': 'Model',
  'agents.field.modelHint': 'Stored verbatim: this string is the pricing key.',
  'agents.field.reasoning': 'Reasoning effort',
  'agents.field.runtime': 'Max runtime (seconds)',
  'agents.field.retry': 'Retry policy',
  'agents.field.attempts': 'Max attempts',
  'agents.field.tools': 'Tools',
  'agents.field.toolsHint': 'Comma separated. A tool outside this list is a capability failure.',
  'agents.field.skills': 'Skills',
  'agents.field.skillsHint': 'Comma separated.',
  'agents.delete': 'Delete',
  'agents.delete.title': 'Delete agent',
  'agents.delete.body': 'Tasks assigned to this agent lose their assignee. This cannot be undone.',
  'agents.delete.confirm': 'Delete',
  'agents.delete.pending': 'Deleting…',
  'agents.delete.failed': 'The agent was not deleted',
  'agents.status.ready': 'READY',
  'agents.status.needsKey': 'NEEDS CREDENTIAL',
  'agents.status.archived': 'ARCHIVED',
  'agents.filter.all': 'All statuses',
  'agents.filter.label': 'Filter by fleet status',
  'agents.filter.empty': 'No agent matches that search',
  'agents.filter.emptyHint': 'Clear the search or set the filter back to all statuses.',
  'agents.footer.showing': 'Showing {0} of {1} agents registered',
  'agents.footer.ready': '{0} ready to assign',
  'agents.footer.archived': '{0} archived (hidden from the assign dropdown)',
  'agents.statusCard.title': '{0} agents in this project',
  'agents.statusCard.hint': '{0} can take a task now · {1} waiting for a provider key',
  'agents.statusCard.ready': '{0} READY',
  'agents.statusCard.needsKey': '{0} NEEDS KEY',
  'agents.statusCard.archived': '{0} ARCHIVED',
  'agents.spec.title': 'What a valid agent needs',
  'agents.spec.badge': '8 required fields',
  'agents.spec.intro': 'Every field below is validated when an agent is registered:',
  'agents.spec.credentialLabel': 'No provider key yet?',
  'agents.spec.credentialBody':
    'An agent can be registered without a provider key. It stays unconfigured until the secret is filled in, and it cannot be assigned work until then.',
  'agents.archive.title': 'What archiving an agent does',
  'agents.archive.badge': 'Reversible',
  'agents.archive.runningLabel': 'Running work finishes:',
  'agents.archive.runningBody':
    'When {0} is archived, a task that is already {1} runs to completion. Its token spend stays in the cost ledger.',
  'agents.archive.assignLabel': 'Hidden from new assignments:',
  'agents.archive.assignBody':
    'An archived agent is filtered out of the assign picker on the board and in the table view, so {0} can never be picked for new work.',
  'agents.archive.assignable': '{0} agents can be assigned right now',
  'agents.archive.hidden': '{0} archived',
  'agents.archive.note': 'Unarchive any time — nothing is deleted.',
  'agents.detail.back': 'Back to agents',
  'agents.detail.title': 'Agent details',
  'agents.detail.statusActive': 'ACTIVE · READY TO ASSIGN',
  'agents.detail.statusArchived': 'ARCHIVED',
  'agents.detail.statusNeedsKey': 'NEEDS PROVIDER CREDENTIAL',
  'agents.detail.id': 'ID',
  'agents.detail.created': 'created',
  'agents.detail.runs': 'Total runs',
  'agents.detail.runsHint': 'Completed runs',
  'agents.detail.cost': 'Total spend',
  'agents.detail.costEstimate': 'Estimate, not a bill',
  'agents.detail.providerTitle': 'Provider & model configuration',
  'agents.detail.providerVerified': 'pricing available',
  'agents.detail.provider': 'Provider',
  'agents.detail.model': 'Model',
  'agents.detail.modelHint': 'Choose a model from the current pricing catalog.',
  'agents.detail.modelUnavailable': 'Model is not in the current catalog',
  'agents.detail.baseUrl': 'Provider base URL',
  'agents.detail.baseUrlHint': 'Required for a custom OpenAI-compatible provider.',
  'agents.detail.runtimeTitle': 'Runtime parameters & tool access',
  'agents.detail.runtimeSubtitle': 'Execution limits',
  'agents.detail.runtime': 'Max runtime',
  'agents.detail.retry': 'Retry policy',
  'agents.detail.reasoning': 'Reasoning effort',
  'agents.detail.attempts': 'Max attempts',
  'agents.detail.tools': 'Allowed tools',
  'agents.detail.skills': 'Skills',
  'agents.detail.tasksTitle': 'Tasks assigned to this agent',
  'agents.detail.tasksEmpty': 'Task history is shown on each board.',
  'agents.detail.save': 'Save changes',
  'agents.detail.saving': 'Saving…',
  'agents.detail.saved': 'Changes saved.',
  'agents.detail.saveFailed': 'Could not save changes',
  'agents.detail.loading': 'Loading agent…',
  'agents.detail.notFound': 'Agent not found',
  'agents.detail.archiveAction': 'Archive agent',
  'agents.detail.unarchiveAction': 'Unarchive agent',
  'agents.detail.archiveHint': 'Archiving hides this agent from new assignments. Running work is not interrupted.',
  'agents.archive.forbidden': 'Only a workspace owner or admin can archive or unarchive an agent.',
  'agents.archive.running': 'This agent still has running work. Let it finish before archiving.',
  'agents.archive.failed': 'Could not change the archive status',
}

const id: Dictionary = {
  'nav.boards': 'Board',
  'nav.projects': 'Project',
  'nav.tasks': 'Task',
  'nav.agents': 'Agent',
  'nav.finops': 'Biaya & Pemakaian',
  'nav.approvals': 'Persetujuan',
  'nav.settings': 'Pengaturan',
  'action.newProject': 'Project baru',
  'action.create': 'Buat',
  'action.cancel': 'Batal',
  'action.retry': 'Ulangi',
  'action.open': 'Buka',
  'field.name': 'Nama',
  'field.slug': 'Slug',
  'field.title': 'Judul',
  'field.description': 'Deskripsi',
  'field.priority': 'Prioritas',
  'field.email': 'Email',
  'field.password': 'Password',
  'field.newPassword': 'Password baru',
  'field.confirmPassword': 'Konfirmasi password baru',
  'empty.projects': 'Belum ada project',
  'empty.boards': 'Belum ada board',
  'empty.tasks': 'Belum ada task',
  'state.loading': 'Memuat…',
  'state.error': 'Ada yang gagal',
  'cost.today': 'Hari ini',
  'cost.runningNow': 'Sedang jalan',
  'auth.tagline': 'Fleet Orchestration',
  'auth.login.title': 'Masuk ke AgentDeck',
  'auth.login.subtitle': 'Masukkan kredensial akun untuk mengakses fleet.',
  'auth.login.submit': 'Log in',
  'auth.login.pending': 'Menghubungkan…',
  'auth.login.forgot': 'Lupa password?',
  'auth.register.title': 'Daftar ke AgentDeck',
  'auth.register.subtitle': 'Mulai orkestrasi armada agen AI Anda dengan akun baru.',
  'auth.register.submit': 'Daftar Akun',
  'auth.register.pending': 'Mendaftarkan…',
  'auth.register.passwordHint': 'min. 8 karakter',
  'auth.register.workspaceBadgeTitle': 'Workspace Personal Otomatis',
  'auth.register.workspaceBadgeText':
    'Workspace personal langsung dibuat tanpa konfigurasi organisasi. Anda dapat langsung membuat project & board pertama Anda.',
  'auth.resetRequest.title': 'Lupa password',
  'auth.resetRequest.subtitle':
    'Masukkan email akun Anda. Kami akan mengirimkan tautan verifikasi untuk menyetel ulang password.',
  'auth.resetRequest.submit': 'Kirim Tautan Reset',
  'auth.resetRequest.pending': 'Mengirim…',
  'auth.resetRequest.sent': 'Kalau email itu terdaftar, tautan reset sedang dikirim.',
  'auth.resetConfirm.title': 'Buat password baru',
  'auth.resetConfirm.subtitle': 'Masukkan kata sandi baru untuk akun Anda. Pastikan memenuhi standar keamanan armada.',
  'auth.resetConfirm.submit': 'Simpan Password Baru',
  'auth.resetConfirm.pending': 'Menyimpan…',
  'auth.resetConfirm.mismatch': 'Dua password tidak sama.',
  'auth.resetConfirm.success': 'Password diperbarui. Semua sesi lain sudah dicabut.',
  'auth.securityNoticeTitle': 'Ketentuan Keamanan (US-AD88 AC2)',
  'auth.resetRequest.notice':
    'Tautan reset mewajibkan password baru minimal 8 karakter. Setelah reset berhasil, seluruh sesi login ({sessions}) pada perangkat lain akan dicabut otomatis.',
  'auth.resetConfirm.notice':
    'Password baru minimal 8 karakter. Setelah reset berhasil, kolom {users.password_hash} diperbarui dan seluruh baris {sessions} milik akun dicabut otomatis.',
  'auth.haveAccount': 'Sudah memiliki akun?',
  'auth.noAccount': 'Belum punya akun?',
  'auth.signIn': 'Masuk',
  'auth.logout': 'Keluar',
  'auth.rememberedPassword': 'Ingat password Anda?',

  // Settings → Members (US-AD04).
  'members.title': 'Anggota & role',
  'members.subtitle': 'Daftar anggota ruang kerja',
  'members.joined': 'Bergabung',
  'members.search': 'Cari nama, email, role…',
  'members.count': '{0} anggota',
  'members.you': 'anda',
  'members.ownerLocked': 'Pemilik penuh',
  'members.empty': 'Belum ada anggota',
  'members.emptyHint': 'Undang lewat email untuk memberi akses ke ruang kerja ini.',
  'members.colUser': 'Pengguna',
  'members.colEmail': 'Email',
  'members.colRole': 'Role',
  'members.colJoined': 'Bergabung',
  'members.colAction': 'Aksi',
  'members.changeRole': 'Ubah role',
  'members.remove': 'Keluarkan',
  'members.invite.cta': 'Undang anggota',
  'members.invite.title': 'Undang anggota',
  'members.invite.description':
    'Anggota langsung ditambahkan dengan role yang dipilih. Alamatnya dikirimi pemberitahuan.',
  'members.invite.submit': 'Kirim undangan',
  'members.invite.pending': 'Mengirim…',
  'members.invite.done': 'Undangan terkirim ke {0} sebagai {1}.',
  'members.invite.failed': 'Undangan gagal dikirim',
  'members.role.title': 'Ubah role',
  'members.role.description': 'Role baru berlaku langsung.',
  'members.role.submit': 'Simpan role',
  'members.role.failed': 'Role gagal diubah',
  'members.role.ownerHint': 'Owner diberikan saat ruang kerja dibuat dan tidak bisa ditetapkan di sini.',
  'members.remove.title': 'Keluarkan anggota',
  'members.remove.description': 'Aksesnya ke ruang kerja ini dicabut. Akunnya tidak dihapus.',
  'members.remove.submit': 'Keluarkan',
  'members.remove.failed': 'Anggota gagal dikeluarkan',
  'role.owner': 'Pemilik',
  'role.admin': 'Admin',
  'role.member': 'Anggota',
  'role.viewer': 'Pengamat',
  'members.loadFailed': 'Daftar anggota gagal dimuat',
  'workspace.title': 'Pengaturan ruang kerja',
  'workspace.subtitle': 'Pengaturan organisasi',
  'workspace.badge': 'Pengaturan',
  'workspace.description': 'Ringkasan ruang kerja aktif. Alur inti dapat diselesaikan tanpa membuka halaman ini.',
  'workspace.soloTitle': 'Alur solo tidak terblokir',
  'workspace.soloText':
    'Pengguna dengan satu ruang kerja dapat membuat project, mendaftarkan agen, dan menjalankan board tanpa membuka settings.',
  'workspace.soloStatus': 'Alur inti B2C aktif',
  'workspace.cleanTitle': 'Tidak ada selector ruang kerja mubazir',
  'workspace.cleanText': 'Saat hanya ada satu ruang kerja, top bar tetap bersih tanpa dropdown satu item.',
  'workspace.cleanStatus': 'Top bar tanpa selector redundan',
  'workspace.switcher': 'Pindah ruang kerja',
  'workspace.summaryTitle': 'Ringkasan ruang kerja aktif',
  'workspace.summaryHint': 'Nilai nyata dari ruang kerja aktif; total resource belum tersedia di milestone ini.',
  'workspace.id': 'ID ruang kerja',
  'workspace.displayName': 'Nama display',
  'workspace.slug': 'Slug',
  'workspace.kind': 'Tipe kepemilikan',
  'workspace.role': 'Role Anda',
  'workspace.editName': 'Ganti nama ruang kerja',
  'workspace.editTitle': 'Ganti nama ruang kerja',
  'workspace.editDescription': 'Slug tetap sama. Perubahan ini dicatat di audit trail ruang kerja.',
  'workspace.save': 'Simpan nama',
  'workspace.savePending': 'Menyimpan…',
  'workspace.resourceTitle': 'Resource terdaftar',
  'workspace.resourceUnavailable':
    'Total resource dan ringkasan biaya hadir bersama endpoint M1/M2 masing-masing. Tidak ada angka placeholder di sini.',
  'workspace.securityTitle': 'Isolasi data org-scoped',
  'workspace.securityText':
    'Project, board, anggota, approval, dan data biaya diminta dalam konteks ruang kerja aktif.',
  'workspace.updateFailed': 'Nama ruang kerja tidak berubah',
  'workspace.updated': 'Nama ruang kerja diperbarui',
  'profile.title': 'Profil akun mandiri',
  'profile.subtitle': 'Identitas dan keanggotaan ruang kerja',
  'profile.description':
    'Identitas Anda diisolasi per sesi login dan dibaca langsung dari endpoint daemon GET /api/v1/auth/me.',
  'profile.menu': 'Buka menu profil',
  'profile.menuOpen': 'Menu profil',
  'profile.account': 'Akun aktif:',
  'profile.accountActive': 'Masuk sebagai {0}',
  'profile.idField': 'id (user id)',
  'profile.idHint': 'Identifier internal daemon',
  'profile.emailField': 'email',
  'profile.emailHint': 'Identitas otentikasi login',
  'profile.nameField': 'name',
  'profile.nameHint': 'Nama display di topbar & drawer',
  'profile.avatarField': 'avatar_user',
  'profile.avatarAuto': 'Otomatis',
  'profile.avatarHint': 'Diturunkan dari nama display oleh server',
  'profile.save': 'Simpan perubahan',
  'profile.savePending': 'Menyimpan…',
  'profile.reset': 'Reset',
  'profile.saved': 'Profil diperbarui',
  'profile.saveFailed': 'Profil tidak berubah',
  'profile.workspacesTitle': 'Keanggotaan ruang kerja',
  'profile.workspacesHint': 'Semua ruang kerja yang diikuti akun ini, beserta role-nya.',
  'profile.workspaceRole': 'Role',
  'profile.workspaceKind': 'Tipe kepemilikan',
  'profile.empty': 'Belum ada keanggotaan ruang kerja untuk akun ini.',
  'profile.roleNote': 'Halaman ini tidak memerlukan peran owner atau admin.',
  'profile.readOnly': 'Read-Only',
  'profile.editable': 'Dapat diubah',

  'boards.new': 'Board baru',
  'boards.create.title': 'Board baru',
  'boards.create.description': 'Board dibuat di dalam project yang Anda pilih, pada ruang kerja aktif.',
  'boards.create.project': 'Project',
  'boards.create.columnsHint': 'Dimulai dengan kolom default: Backlog, Ready, Running, Review, Done.',
  'boards.create.submit': 'Buat',
  'boards.create.pending': 'Membuat…',
  'boards.create.failed': 'Board tidak dibuat',
  'boards.empty': 'Belum ada board',
  'boards.emptyHint': 'Buat board pertama di ruang kerja ini untuk mulai memindahkan pekerjaan.',
  'boards.createFirst': 'Buat board pertama Anda',
  'boards.noProjects': 'Belum ada project',
  'boards.noProjectsHint': 'Board berada di dalam project. Buat project dulu di halaman Projects.',
  'agents.title': 'Agent Registry',
  'agents.new': 'Daftarkan agent',
  'agents.search': 'Cari agent, model, atau skill...',
  'agents.col.agent': 'Agent / ID',
  'agents.col.provider': 'Provider',
  'agents.col.model': 'Model',
  'agents.col.reasoning': 'Reasoning',
  'agents.col.status': 'Status fleet',
  'agents.col.runtime': 'Runtime & retry',
  'agents.col.tools': 'Tools / skill',
  'agents.col.actions': 'Aksi',
  'agents.count': 'agent terdaftar',
  'agents.empty': 'Belum ada agent di project ini',
  'agents.emptyHint': 'Daftarkan satu supaya task punya runner.',
  'agents.noProjects': 'Belum ada project',
  'agents.noProjectsHint': 'Agent dimiliki sebuah project. Buat project dulu di halaman Projects.',
  'agents.create.title': 'Daftarkan agent',
  'agents.create.description': 'Agent jadi sumber batas retry dan runtime untuk setiap task yang dijalankannya.',
  'agents.create.submit': 'Simpan',
  'agents.create.pending': 'Menyimpan…',
  'agents.create.failed': 'Agent tidak terdaftar',
  'agents.field.provider': 'Provider',
  'agents.field.model': 'Model',
  'agents.field.modelHint': 'Disimpan apa adanya: string ini jadi kunci harga.',
  'agents.field.reasoning': 'Reasoning effort',
  'agents.field.runtime': 'Max runtime (detik)',
  'agents.field.retry': 'Kebijakan retry',
  'agents.field.attempts': 'Max percobaan',
  'agents.field.tools': 'Tools',
  'agents.field.toolsHint': 'Pisahkan dengan koma. Tool di luar daftar ini jadi kegagalan capability.',
  'agents.field.skills': 'Skill',
  'agents.field.skillsHint': 'Pisahkan dengan koma.',
  'agents.delete': 'Hapus',
  'agents.delete.title': 'Hapus agent',
  'agents.delete.body': 'Task yang ditugaskan ke agent ini kehilangan assignee-nya. Tidak bisa dibatalkan.',
  'agents.delete.confirm': 'Hapus',
  'agents.delete.pending': 'Menghapus…',
  'agents.delete.failed': 'Agent tidak dihapus',
  'agents.status.ready': 'SIAP',
  'agents.status.needsKey': 'BUTUH KREDENSIAL',
  'agents.status.archived': 'DIARSIP',
  'agents.filter.all': 'Semua Status',
  'agents.filter.label': 'Filter berdasarkan status fleet',
  'agents.filter.empty': 'Tidak ada agent yang cocok',
  'agents.filter.emptyHint': 'Kosongkan pencarian atau kembalikan filter ke semua status.',
  'agents.footer.showing': 'Menampilkan {0} dari {1} agent terdaftar',
  'agents.footer.ready': '{0} siap di-assign',
  'agents.footer.archived': '{0} diarsip (tidak muncul di dropdown assign)',
  'agents.statusCard.title': '{0} agent di project ini',
  'agents.statusCard.hint': '{0} siap menerima task · {1} menunggu kredensial provider',
  'agents.statusCard.ready': '{0} SIAP',
  'agents.statusCard.needsKey': '{0} BUTUH KREDENSIAL',
  'agents.statusCard.archived': '{0} DIARSIP',
  'agents.spec.title': 'Syarat agent yang valid',
  'agents.spec.badge': '8 field wajib',
  'agents.spec.intro': 'Semua field di bawah divalidasi saat agent didaftarkan:',
  'agents.spec.credentialLabel': 'Belum punya kredensial provider?',
  'agents.spec.credentialBody':
    'Agent boleh didaftarkan tanpa kredensial provider. Statusnya belum terkonfigurasi sampai kredensial diisi, dan selama itu agent belum bisa ditugaskan.',
  'agents.archive.title': 'Efek mengarsipkan agent',
  'agents.archive.badge': 'Bisa dibatalkan',
  'agents.archive.runningLabel': 'Task yang sedang jalan tetap tuntas:',
  'agents.archive.runningBody':
    'Saat {0} diarsip, task yang berstatus {1} tetap dikerjakan sampai selesai. Pemakaian tokennya tetap tercatat di ledger biaya.',
  'agents.archive.assignLabel': 'Hilang dari penugasan baru:',
  'agents.archive.assignBody':
    'Agent yang diarsip disaring keluar dari pemilih assign di board dan di table view, jadi {0} tidak akan pernah terpilih untuk task baru.',
  'agents.archive.assignable': '{0} agent bisa ditugaskan sekarang',
  'agents.archive.hidden': '{0} diarsip',
  'agents.archive.note': 'Bisa dibatalkan kapan saja — tidak ada yang dihapus.',
  'agents.detail.back': 'Kembali ke agent',
  'agents.detail.title': 'Detail agent',
  'agents.detail.statusActive': 'AKTIF · SIAP DITUGASKAN',
  'agents.detail.statusArchived': 'DIARSIP',
  'agents.detail.statusNeedsKey': 'BUTUH KREDENSIAL PROVIDER',
  'agents.detail.id': 'ID',
  'agents.detail.created': 'dibuat',
  'agents.detail.runs': 'Total run',
  'agents.detail.runsHint': 'Run selesai',
  'agents.detail.cost': 'Total biaya',
  'agents.detail.costEstimate': 'Estimasi, bukan tagihan',
  'agents.detail.providerTitle': 'Konfigurasi provider & model',
  'agents.detail.providerVerified': 'harga tersedia',
  'agents.detail.provider': 'Provider',
  'agents.detail.model': 'Model',
  'agents.detail.modelHint': 'Pilih model dari katalog harga terbaru.',
  'agents.detail.modelUnavailable': 'Model tidak ada di katalog saat ini',
  'agents.detail.baseUrl': 'URL dasar provider',
  'agents.detail.baseUrlHint': 'Wajib untuk provider OpenAI-compatible kustom.',
  'agents.detail.runtimeTitle': 'Parameter runtime & akses tools',
  'agents.detail.runtimeSubtitle': 'Batas eksekusi',
  'agents.detail.runtime': 'Runtime maksimum',
  'agents.detail.retry': 'Kebijakan retry',
  'agents.detail.reasoning': 'Reasoning effort',
  'agents.detail.attempts': 'Maks percobaan',
  'agents.detail.tools': 'Tools yang diizinkan',
  'agents.detail.skills': 'Skill',
  'agents.detail.tasksTitle': 'Task yang ditugaskan ke agent ini',
  'agents.detail.tasksEmpty': 'Riwayat task tersedia di masing-masing board.',
  'agents.detail.save': 'Simpan perubahan',
  'agents.detail.saving': 'Menyimpan…',
  'agents.detail.saved': 'Perubahan tersimpan.',
  'agents.detail.saveFailed': 'Perubahan tidak tersimpan',
  'agents.detail.loading': 'Memuat agent…',
  'agents.detail.notFound': 'Agent tidak ditemukan',
  'agents.detail.archiveAction': 'Arsipkan agent',
  'agents.detail.unarchiveAction': 'Batal arsip agent',
  'agents.detail.archiveHint':
    'Arsip menyembunyikan agent dari penugasan baru. Task yang sedang berjalan tidak dihentikan.',
  'agents.archive.forbidden': 'Hanya owner atau admin workspace yang dapat mengarsipkan atau membatalkan arsip agent.',
  'agents.archive.running': 'Agent ini masih punya task yang berjalan. Tunggu sampai selesai sebelum mengarsipkan.',
  'agents.archive.failed': 'Status arsip tidak berubah',
}

const DICTIONARIES: Record<Lang, Dictionary> = { en, id }
const cache = new Map<Lang, Promise<Dictionary>>()

export function loadDictionary(lang: Lang): Promise<Dictionary> {
  const cached = cache.get(lang)
  if (cached) return cached
  const promise = Promise.resolve(DICTIONARIES[lang])
  cache.set(lang, promise)
  return promise
}

/** Synchronous lookup for non-render code (listeners, formatters, tests). */
export function translate(lang: Lang, key: keyof Dictionary): string {
  return DICTIONARIES[lang][key]
}
