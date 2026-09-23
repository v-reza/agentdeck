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
  // Sidebar settings groups (design 47-providers / 38-members). These links are
  // the only route into /settings/*; the rail's gear only reaches workspace.
  'sidebar.nav': string
  'sidebar.accountGroup': string
  'sidebar.integrationsGroup': string
  // API keys and webhooks tabs. Both screens are placeholders until their
  // endpoints land, but the sidebar names them, so the copy must exist.
  'apiKeys.title': string
  'webhooks.title': string
  // Settings → Providers (US-AD109). The per-workspace credential registry.
  'providers.title': string
  'providers.subtitle': string
  'providers.count': string
  'providers.add': string
  'providers.colName': string
  'providers.colProtocol': string
  'providers.colBaseUrl': string
  'providers.colCredential': string
  'providers.colModels': string
  'providers.colVerified': string
  'providers.colSync': string
  'providers.colDefault': string
  'providers.colActions': string
  'providers.encrypted': string
  'providers.verified': string
  'providers.unverified': string
  'providers.default': string
  'providers.stale': string
  'providers.neverFetched': string
  'providers.test': string
  'providers.refresh': string
  'providers.empty': string
  'providers.emptyHint': string
  'providers.loadFailed': string
  // Create/edit dialog. The credential is write-only (AC2): on edit the field
  // starts empty and leaving it empty keeps the stored key.
  'providers.form.create': string
  'providers.form.edit': string
  'providers.form.name': string
  'providers.form.protocol': string
  'providers.form.baseUrl': string
  'providers.form.baseUrlHint': string
  'providers.form.apiKey': string
  'providers.form.apiKeyHint': string
  'providers.form.apiKeyOptional': string
  'providers.form.keepKey': string
  'providers.form.isDefault': string
  'providers.form.submit': string
  'providers.form.pending': string
  'providers.form.failed': string
  // Delete dialog and the 409 that names the blocking agents (AC5).
  'providers.delete.title': string
  'providers.delete.body': string
  'providers.delete.submit': string
  'providers.delete.failed': string
  'providers.inUse.title': string
  'providers.inUse.body': string
  // Verify (AC3) and manual model refresh (AC7) outcomes.
  'providers.verifyOk': string
  'providers.verifyFailed': string
  'providers.modelsOk': string
  'providers.modelsFailed': string
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
  'profile.verified': string
  'profile.selfManaged': string
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
  // Register modal sections (26-agent-form, US-AD96).
  'agents.create.section.identity': string
  'agents.create.section.provider': string
  'agents.create.section.credential': string
  'agents.create.section.runtime': string
  'agents.create.required': string
  'agents.create.optional': string
  'agents.create.fromRegistry': string
  'agents.create.credentialFromProvider': string
  'agents.create.credentialViaProvider': string
  'agents.create.noProvider': string
  'agents.create.noProviderCta': string
  'agents.create.modelPlaceholder': string
  'agents.create.modelNotInCatalog': string
  'agents.create.catalogLoading': string
  // The write-only credential field (US-AD96 AC2/AC4/AC5).
  'agents.cred.field': string
  'agents.cred.fieldHint': string
  'agents.cred.stored': string
  'agents.cred.missing': string
  'agents.cred.masked': string
  'agents.cred.optional': string
  'agents.cred.test': string
  'agents.cred.testing': string
  'agents.cred.testOk': string
  'agents.cred.testRefused': string
  'agents.cred.testFailed': string
  'agents.cred.testNeedsSave': string
  // The 420px credential panel (27-agent-provider-key, US-AD86).
  'agents.key.title': string
  'agents.key.newAgent': string
  'agents.key.newAgentHint': string
  'agents.key.target': string
  'agents.key.provider': string
  'agents.key.unsaved': string
  'agents.key.save': string
  'agents.key.saving': string
  'agents.key.saved': string
  'agents.key.failed': string
  'agents.key.revoke': string
  'agents.key.revoking': string
  'agents.key.encryption': string
  'agents.key.empty': string
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
  'agents.detail.providerOpenAI': string
  'agents.detail.providerAnthropic': string
  'agents.detail.providerDeepseek': string
  'agents.detail.providerCustom': string
  'agents.detail.model': string
  'agents.detail.modelHint': string
  'agents.detail.noProvider': string
  'agents.detail.runtimeTitle': string
  'agents.detail.runtimeSubtitle': string
  'agents.detail.runtime': string
  'agents.detail.retry': string
  'agents.detail.reasoning': string
  'agents.detail.attempts': string
  'agents.detail.tools': string
  'agents.detail.skills': string
  'agents.detail.assignmentBoard': string
  'agents.detail.pickerFilter': string
  'agents.detail.pickerFilterActive': string
  'agents.detail.pickerReady': string
  'agents.detail.pickerNeedsKey': string
  'agents.detail.taskColId': string
  'agents.detail.taskColTitle': string
  'agents.detail.taskColStatus': string
  'agents.detail.taskColCost': string
  'agents.detail.assignmentTitle': string
  'agents.detail.assignmentLead': string
  'agents.detail.assignmentTasksTitle': string
  'agents.detail.assignmentTasksHint': string
  'agents.detail.assignmentEmpty': string
  'agents.detail.assignmentRunning': string
  'agents.detail.assignmentFooter': string
  'agents.detail.tasksTitle': string
  'agents.detail.tasksEmpty': string
  'agents.detail.pickerHint': string
  'agents.detail.pickerEmpty': string
  'agents.detail.pickerHidden': string
  'agents.detail.agent': string
  'agents.detail.save': string
  'agents.detail.saving': string
  'agents.detail.saved': string
  'agents.detail.saveFailed': string
  'agents.detail.loading': string
  'agents.detail.notFound': string
  'agents.detail.archiveAction': string
  'agents.detail.unarchiveAction': string
  'agents.archive.forbidden': string
  'agents.archive.running': string
  'agents.archive.failed': string
  'agents.detail.lifecycleTitle': string
  'agents.detail.lifecycleFooterActive': string
  'agents.detail.lifecycleFooterArchived': string
  'agents.detail.assignmentState': string
  'agents.detail.rule1Title': string
  'agents.detail.rule1Body': string
  'agents.detail.rule2Title': string
  'agents.detail.rule2Body': string
  'agents.detail.toolsGranted': string
  'agents.detail.skillsInUse': string
  'agents.detail.toolsHint': string
  'agents.detail.skillsHint': string
  'agents.detail.gatePolicy': string
  'agents.detail.gateGated': string
  'agents.detail.gateNone': string
  'agents.detail.priceTitle': string
  'agents.detail.pricePriced': string
  'agents.detail.priceUnpriced': string
  'agents.detail.priceInput': string
  'agents.detail.priceOutput': string
  'agents.detail.priceCached': string
  'agents.detail.priceVersion': string
  'agents.detail.priceDisclaimer': string
  'agents.detail.priceLoading': string
  'agents.detail.priceLoadingBody': string
  'agents.detail.priceNoProvider': string
  'agents.detail.priceUnpricedBody': string
  'agents.detail.priceNoModel': string
  'agents.detail.providerHint': string
  'agents.detail.minutes': string
  'agents.detail.notFoundHint': string
  'agents.detail.saveHint': string
  'agents.detail.skillsEmpty': string
  'agents.detail.justNow': string
  'agents.detail.minutesAgo': string
  'agents.detail.hoursAgo': string
  'agents.detail.daysAgo': string
  'agents.detail.never': string
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
  'sidebar.nav': 'Navigation',
  'sidebar.accountGroup': 'Account & team',
  'sidebar.integrationsGroup': 'Integrations & credentials',
  'apiKeys.title': 'API keys',
  'webhooks.title': 'Webhooks',
  // Settings → Providers (US-AD109).
  'providers.title': 'LLM providers',
  'providers.subtitle': 'Credentials registered once per workspace',
  'providers.count': '{0} providers',
  'providers.add': 'Add provider',
  'providers.colName': 'Name',
  'providers.colProtocol': 'Protocol',
  'providers.colBaseUrl': 'Base URL',
  'providers.colCredential': 'Credential',
  'providers.colModels': 'Models',
  'providers.colVerified': 'Verification',
  'providers.colSync': 'Model sync',
  'providers.colDefault': 'Default',
  'providers.colActions': 'Actions',
  'providers.encrypted': 'encrypted',
  'providers.verified': 'Verified',
  'providers.unverified': 'Not tested',
  'providers.default': 'Default',
  'providers.stale': '{0} (stale)',
  'providers.neverFetched': 'never fetched',
  'providers.test': 'Test',
  'providers.refresh': 'Fetch models',
  'providers.empty': 'No providers yet',
  'providers.emptyHint': 'Register one to stop pasting a base URL and key into every agent.',
  'providers.loadFailed': 'The provider list could not be loaded',
  'providers.form.create': 'Add provider',
  'providers.form.edit': 'Edit provider',
  'providers.form.name': 'Provider name',
  'providers.form.protocol': 'Protocol',
  'providers.form.baseUrl': 'Base URL',
  'providers.form.baseUrlHint':
    'If AgentDeck runs in a container, use host.docker.internal to reach an endpoint on your machine — localhost means the container itself.',
  'providers.form.apiKey': 'API key',
  'providers.form.apiKeyHint': 'Stored encrypted. It is never shown again after saving.',
  'providers.form.apiKeyOptional': 'Optional — a local endpoint that checks nothing needs no key.',
  'providers.form.keepKey': 'Leave empty to keep the stored credential.',
  'providers.form.isDefault': 'Make this the workspace default',
  'providers.form.submit': 'Save provider',
  'providers.form.pending': 'Saving…',
  'providers.form.failed': 'The provider could not be saved',
  'providers.delete.title': 'Delete provider',
  'providers.delete.body': 'Agents using this provider lose their endpoint. This cannot be undone.',
  'providers.delete.submit': 'Delete',
  'providers.delete.failed': 'The provider could not be deleted',
  'providers.inUse.title': 'This provider is still in use',
  'providers.inUse.body': '{0} agents still point at it. Repoint them first: {1}',
  'providers.verifyOk': 'Credential verified',
  'providers.verifyFailed': 'The credential test failed',
  'providers.modelsOk': 'Model list refreshed',
  'providers.modelsFailed': 'The model list could not be fetched',
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
  'profile.verified': 'Verified',
  'profile.selfManaged': 'Self-managed account',
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
  'agents.col.agent': 'Agent',
  'agents.col.provider': 'Provider',
  'agents.col.model': 'Model',
  'agents.col.reasoning': 'Reasoning',
  'agents.col.status': 'Status',
  'agents.col.runtime': 'Runtime',
  'agents.col.tools': 'Tools',
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
  'agents.create.section.identity': 'Agent identity',
  'agents.create.section.provider': 'Provider & model',
  'agents.create.section.credential': 'Provider credential',
  'agents.create.section.runtime': 'Runtime & access',
  'agents.create.required': 'Required',
  'agents.create.optional': 'Optional',
  'agents.create.fromRegistry': 'From the workspace registry',
  'agents.create.credentialFromProvider': 'Held by the provider',
  'agents.create.credentialViaProvider':
    'The endpoint and the API key come from the selected provider, so there is nothing to enter here.',
  'agents.create.noProvider': 'This workspace has no provider yet, and an agent cannot be registered without one.',
  'agents.create.noProviderCta': 'Add a provider',
  'agents.create.modelPlaceholder': 'model name at your endpoint',
  'agents.create.modelNotInCatalog':
    'That model is not in the pricing catalog, so its cost could not be estimated. Pick one from the list, or switch the provider to a custom endpoint.',
  'agents.create.catalogLoading': 'The model list is still loading. Try again in a moment.',
  'agents.cred.field': 'Provider API key',
  'agents.cred.fieldHint':
    'Write-only: the key is encrypted before it is stored and no endpoint ever returns it. Leaving it empty registers the agent without a credential — it is then not ready to take work.',
  'agents.cred.stored': 'CREDENTIAL STORED',
  'agents.cred.missing': 'NO CREDENTIAL',
  'agents.cred.masked': 'masked',
  'agents.cred.optional': 'Optional. Rotating replaces the stored key; no history is kept.',
  'agents.cred.test': 'Test credential',
  'agents.cred.testing': 'Testing…',
  'agents.cred.testOk': 'The provider accepted the credential.',
  'agents.cred.testRefused': 'The provider refused the credential.',
  'agents.cred.testFailed': 'The credential could not be tested',
  'agents.cred.testNeedsSave': 'Save the agent first — the test calls the provider through it.',
  'agents.key.title': 'Provider credential',
  'agents.key.newAgent': 'New agent',
  'agents.key.newAgentHint':
    'This agent has no id yet, so there is nowhere to store a key. Fill in the credential in the form instead — it is stored as part of the save.',
  'agents.key.target': 'Agent',
  'agents.key.provider': 'Provider',
  'agents.key.unsaved': 'not saved yet',
  'agents.key.save': 'Encrypt & save',
  'agents.key.saving': 'Saving…',
  'agents.key.saved': 'Credential saved.',
  'agents.key.failed': 'The credential was not saved',
  'agents.key.revoke': 'Revoke',
  'agents.key.revoking': 'Revoking…',
  'agents.key.encryption':
    'The key is sealed with AES-256-GCM before it reaches the database. The plaintext is never logged and never returned by a read.',
  'agents.key.empty': 'Type the credential first.',
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
  'agents.detail.providerOpenAI': 'openai ([OI]-compatible API)',
  'agents.detail.providerAnthropic': 'anthropic (Claude series)',
  'agents.detail.providerDeepseek': 'deepseek (Coder / V3)',
  'agents.detail.providerCustom': 'custom (self-hosted endpoint)',
  'agents.detail.model': 'Model',
  'agents.detail.modelHint': 'Choose a model from the current pricing catalog.',
  'agents.detail.noProvider': 'No provider — the deployment default',
  'agents.detail.runtimeTitle': 'Runtime parameters & tool access',
  'agents.detail.runtimeSubtitle': 'Execution limits',
  'agents.detail.runtime': 'Max runtime',
  'agents.detail.retry': 'Retry policy',
  'agents.detail.reasoning': 'Reasoning effort',
  'agents.detail.attempts': 'Max attempts',
  'agents.detail.tools': 'Allowed tools',
  'agents.detail.skills': 'Skills',
  'agents.detail.taskColId': 'Task ID',
  'agents.detail.pickerFilter': 'Filter Active Only',
  'agents.detail.pickerFilterActive': 'ACTIVE',
  'agents.detail.pickerReady': 'READY',
  'agents.detail.pickerNeedsKey': 'NO KEY',
  'agents.detail.taskColTitle': 'Task',
  'agents.detail.taskColStatus': 'Status',
  'agents.detail.taskColCost': 'Cost',
  'agents.detail.assignmentTitle': 'Assignment & running work',
  'agents.detail.assignmentLead': 'Assignment rules for this agent.',
  'agents.detail.assignmentBoard': 'Board: {0}',
  'agents.detail.assignmentRunning': '{0} TASK RUNNING',
  'agents.detail.assignmentTasksTitle': 'Tasks this agent holds',
  'agents.detail.assignmentTasksHint': 'A running task is never cut off by archiving — it finishes normally.',
  'agents.detail.assignmentEmpty': 'No task has been assigned to this agent yet.',
  'agents.detail.assignmentFooter': 'Archiving hides the agent from the picker. Work already running still finishes.',
  'agents.detail.tasksTitle': 'Tasks assigned to this agent',
  'agents.detail.tasksEmpty': 'Task history is shown on each board.',
  'agents.detail.pickerHint': 'Only agents that are not archived appear here.',
  'agents.detail.pickerEmpty': 'No board to preview — this agent holds no task yet.',
  'agents.detail.pickerHidden': '{0} archived, hidden from the picker',
  'agents.detail.agent': 'agent',
  'agents.detail.save': 'Save changes',
  'agents.detail.saving': 'Saving…',
  'agents.detail.saved': 'Changes saved.',
  'agents.detail.saveFailed': 'Could not save changes',
  'agents.detail.loading': 'Loading agent…',
  'agents.detail.notFound': 'Agent not found',
  'agents.detail.archiveAction': 'Archive agent',
  'agents.detail.unarchiveAction': 'Unarchive agent',
  'agents.archive.forbidden': 'Only a workspace owner or admin can archive or unarchive an agent.',
  'agents.archive.running': 'This agent still has running work. Let it finish before archiving.',
  'agents.archive.failed': 'Could not change the archive status',
  'agents.detail.lifecycleTitle': 'Assignment lifecycle',
  'agents.detail.lifecycleFooterActive': 'Enforced: runs finish · out of the assign picker',
  'agents.detail.lifecycleFooterArchived': 'Enforced: runs finish · hidden from new assignments',
  'agents.detail.assignmentState': 'Assignment state',
  'agents.detail.rule1Title': 'Running work is never cut off',
  'agents.detail.rule1Body':
    'A task that is already in progress still finishes, and its cost is still written to the ledger.',
  'agents.detail.rule2Title': 'Out of the assignment picker',
  'agents.detail.rule2Body':
    'An archived agent is filtered out of the assign picker on the Kanban board and the table view. Its row stays here so it can be brought back.',
  'agents.detail.toolsGranted': 'Tools granted',
  'agents.detail.skillsInUse': 'Skills in use',
  'agents.detail.toolsHint': 'Tools are a closed list — a name outside it is refused.',
  'agents.detail.skillsHint': 'An agent only reads a skill; it never writes one.',
  'agents.detail.gatePolicy': 'Approval gate',
  'agents.detail.gateGated': 'require (gated)',
  'agents.detail.gateNone': 'not granted',
  'agents.detail.priceTitle': 'Estimated rate',
  'agents.detail.pricePriced': 'pricing table',
  'agents.detail.priceUnpriced': 'no price entry',
  'agents.detail.priceInput': 'Input / 1M tokens',
  'agents.detail.priceOutput': 'Output / 1M tokens',
  'agents.detail.priceCached': 'Cached / 1M tokens',
  'agents.detail.priceVersion': 'price table v{0}',
  'agents.detail.priceDisclaimer':
    'Rates come from the internal AgentDeck pricing table, not a provider invoice. Actual spend is on each provider dashboard.',
  'agents.detail.providerHint': 'A model outside the catalog is stored as-is, but cannot be priced.',
  'agents.detail.priceLoading': 'checking',
  'agents.detail.priceLoadingBody': 'Reading the pricing table for this model…',
  'agents.detail.priceNoProvider': 'This agent has no provider of its own, so no model is pinned to it yet.',
  'agents.detail.priceUnpricedBody': 'No price entry for {0}.',
  'agents.detail.priceNoModel': 'the chosen model',
  'agents.detail.minutes': '({0} min)',
  'agents.detail.notFoundHint': 'It may have been deleted, or it belongs to another workspace.',
  'agents.detail.saveHint': 'The save sends this whole profile; the pickers above only choose its values.',
  'agents.detail.skillsEmpty': 'No skill in this workspace yet',
  'agents.detail.justNow': 'just now',
  'agents.detail.minutesAgo': '{0}m ago',
  'agents.detail.hoursAgo': '{0}h ago',
  'agents.detail.daysAgo': '{0}d ago',
  'agents.detail.never': 'never',
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
  'sidebar.nav': 'Navigasi Utama',
  'sidebar.accountGroup': 'Pengaturan Akun & Tim',
  'sidebar.integrationsGroup': 'Integrasi & Kredensial',
  'apiKeys.title': 'API Keys',
  'webhooks.title': 'Webhooks',
  // Settings → Providers (US-AD109).
  'providers.title': 'Provider LLM',
  'providers.subtitle': 'Kredensial didaftarkan sekali per ruang kerja',
  'providers.count': '{0} provider',
  'providers.add': 'Tambah provider',
  'providers.colName': 'Nama',
  'providers.colProtocol': 'Protokol',
  'providers.colBaseUrl': 'Base URL',
  'providers.colCredential': 'Kredensial',
  'providers.colModels': 'Model',
  'providers.colVerified': 'Verifikasi',
  'providers.colSync': 'Sinkronisasi model',
  'providers.colDefault': 'Default',
  'providers.colActions': 'Aksi',
  'providers.encrypted': 'terenkripsi',
  'providers.verified': 'Terverifikasi',
  'providers.unverified': 'Belum diuji',
  'providers.default': 'Default',
  'providers.stale': '{0} (kedaluwarsa)',
  'providers.neverFetched': 'belum pernah ditarik',
  'providers.test': 'Uji',
  'providers.refresh': 'Tarik model',
  'providers.empty': 'Belum ada provider',
  'providers.emptyHint': 'Daftarkan satu supaya tidak menyalin base URL dan key ke tiap agent.',
  'providers.loadFailed': 'Daftar provider gagal dimuat',
  'providers.form.create': 'Tambah provider',
  'providers.form.edit': 'Ubah provider',
  'providers.form.name': 'Nama provider',
  'providers.form.protocol': 'Protokol',
  'providers.form.baseUrl': 'Base URL',
  'providers.form.baseUrlHint':
    'Kalau AgentDeck jalan di dalam container, pakai host.docker.internal untuk menjangkau endpoint di mesin Anda — localhost berarti container itu sendiri.',
  'providers.form.apiKey': 'API key',
  'providers.form.apiKeyHint': 'Disimpan terenkripsi. Tidak pernah ditampilkan lagi setelah disimpan.',
  'providers.form.apiKeyOptional': 'Opsional — endpoint lokal yang tidak memeriksa apa pun tidak butuh key.',
  'providers.form.keepKey': 'Kosongkan untuk mempertahankan kredensial yang tersimpan.',
  'providers.form.isDefault': 'Jadikan default ruang kerja',
  'providers.form.submit': 'Simpan provider',
  'providers.form.pending': 'Menyimpan…',
  'providers.form.failed': 'Provider gagal disimpan',
  'providers.delete.title': 'Hapus provider',
  'providers.delete.body': 'Agent yang memakai provider ini kehilangan endpoint-nya. Tidak bisa dibatalkan.',
  'providers.delete.submit': 'Hapus',
  'providers.delete.failed': 'Provider gagal dihapus',
  'providers.inUse.title': 'Provider ini masih dipakai',
  'providers.inUse.body': '{0} agent masih menunjuk ke sini. Pindahkan dulu: {1}',
  'providers.verifyOk': 'Kredensial terverifikasi',
  'providers.verifyFailed': 'Uji kredensial gagal',
  'providers.modelsOk': 'Daftar model diperbarui',
  'providers.modelsFailed': 'Daftar model gagal ditarik',
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
  'profile.verified': 'Terverifikasi',
  'profile.selfManaged': 'Akun mandiri',
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
  'agents.col.agent': 'Agent',
  'agents.col.provider': 'Provider',
  'agents.col.model': 'Model',
  'agents.col.reasoning': 'Reasoning',
  'agents.col.status': 'Status',
  'agents.col.runtime': 'Runtime',
  'agents.col.tools': 'Tools',
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
  'agents.create.section.identity': 'Identitas agent',
  'agents.create.section.provider': 'Provider & model',
  'agents.create.section.credential': 'Kredensial provider',
  'agents.create.section.runtime': 'Runtime & akses',
  'agents.create.required': 'Wajib diisi',
  'agents.create.optional': 'Opsional',
  'agents.create.fromRegistry': 'Dari registry ruang kerja',
  'agents.create.credentialFromProvider': 'Dipegang provider',
  'agents.create.credentialViaProvider':
    'Endpoint dan API key ikut provider yang dipilih, jadi tidak ada yang perlu diisi di sini.',
  'agents.create.noProvider': 'Ruang kerja ini belum punya provider, dan agent tidak bisa didaftarkan tanpa satu.',
  'agents.create.noProviderCta': 'Tambah provider',
  'agents.create.modelPlaceholder': 'nama model di endpoint Anda',
  'agents.create.modelNotInCatalog':
    'Model itu tidak ada di katalog harga, jadi biayanya tidak bisa diestimasi. Pilih dari daftar, atau ganti provider ke endpoint sendiri.',
  'agents.create.catalogLoading': 'Daftar model masih dimuat. Coba lagi sebentar lagi.',
  'agents.cred.field': 'API key provider',
  'agents.cred.fieldHint':
    'Tulis-saja: kunci dienkripsi sebelum disimpan dan tidak ada endpoint yang mengembalikannya. Kalau dibiarkan kosong, agent terdaftar tanpa kredensial — statusnya belum siap menerima task.',
  'agents.cred.stored': 'KREDENSIAL TERSIMPAN',
  'agents.cred.missing': 'BELUM ADA KREDENSIAL',
  'agents.cred.masked': 'ter-mask',
  'agents.cred.optional': 'Opsional. Mengganti kunci menimpa yang lama; tidak ada riwayat yang disimpan.',
  'agents.cred.test': 'Uji kredensial',
  'agents.cred.testing': 'Menguji…',
  'agents.cred.testOk': 'Provider menerima kredensial ini.',
  'agents.cred.testRefused': 'Provider menolak kredensial ini.',
  'agents.cred.testFailed': 'Kredensial tidak bisa diuji',
  'agents.cred.testNeedsSave': 'Simpan agent dulu — uji ini memanggil provider lewat agent tersebut.',
  'agents.key.title': 'Kredensial provider',
  'agents.key.newAgent': 'Agent baru',
  'agents.key.newAgentHint':
    'Agent ini belum punya id, jadi belum ada tempat untuk menyimpan kunci. Isi kredensialnya di formulir — kunci disimpan bersamaan dengan penyimpanan agent.',
  'agents.key.target': 'Agent',
  'agents.key.provider': 'Provider',
  'agents.key.unsaved': 'belum tersimpan',
  'agents.key.save': 'Enkripsi & simpan',
  'agents.key.saving': 'Menyimpan…',
  'agents.key.saved': 'Kredensial tersimpan.',
  'agents.key.failed': 'Kredensial tidak tersimpan',
  'agents.key.revoke': 'Cabut',
  'agents.key.revoking': 'Mencabut…',
  'agents.key.encryption':
    'Kunci disegel dengan AES-256-GCM sebelum masuk database. Plaintext-nya tidak pernah dicatat di log dan tidak pernah dikembalikan oleh pembacaan mana pun.',
  'agents.key.empty': 'Isi kredensialnya dulu.',
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
  'agents.detail.providerOpenAI': 'openai (API kompatibel [OI])',
  'agents.detail.providerAnthropic': 'anthropic (seri Claude)',
  'agents.detail.providerDeepseek': 'deepseek (Coder / V3)',
  'agents.detail.providerCustom': 'kustom (endpoint sendiri)',
  'agents.detail.model': 'Model',
  'agents.detail.modelHint': 'Pilih model dari katalog harga terbaru.',
  'agents.detail.noProvider': 'Belum ada provider — pakai default deployment',
  'agents.detail.runtimeTitle': 'Parameter runtime & akses tools',
  'agents.detail.runtimeSubtitle': 'Batas eksekusi',
  'agents.detail.runtime': 'Runtime maksimum',
  'agents.detail.retry': 'Kebijakan retry',
  'agents.detail.reasoning': 'Reasoning effort',
  'agents.detail.attempts': 'Maks percobaan',
  'agents.detail.tools': 'Tools yang diizinkan',
  'agents.detail.skills': 'Skill',
  'agents.detail.taskColId': 'Task ID',
  'agents.detail.assignmentTitle': 'Penugasan & pekerjaan berjalan',
  'agents.detail.assignmentLead': 'Aturan penugasan agent ini.',
  'agents.detail.assignmentBoard': 'Board: {0}',
  'agents.detail.assignmentRunning': '{0} TASK RUNNING',
  'agents.detail.assignmentTasksTitle': 'Task yang dipegang agent ini',
  'agents.detail.assignmentTasksHint': 'Task yang sedang jalan tidak pernah diputus oleh arsip — dia tuntas normal.',
  'agents.detail.assignmentEmpty': 'Belum ada task yang ditugaskan ke agent ini.',
  'agents.detail.assignmentFooter':
    'Mengarsipkan menyembunyikan agent dari picker di kiri. Pekerjaan yang sudah jalan tetap tuntas.',
  'agents.detail.pickerHint': 'Cuma agent yang tidak diarsip yang muncul di sini.',
  'agents.detail.pickerEmpty': 'Belum ada board buat dipratinjau — agent ini belum pegang task.',
  'agents.detail.pickerFilter': 'Filter Active Only',
  'agents.detail.pickerFilterActive': 'ACTIVE',
  'agents.detail.pickerReady': 'READY',
  'agents.detail.pickerNeedsKey': 'TANPA KEY',
  'agents.detail.taskColTitle': 'Task',
  'agents.detail.taskColStatus': 'Status',
  'agents.detail.taskColCost': 'Biaya',
  'agents.detail.pickerHidden': '{0} diarsip, disembunyikan dari picker',
  'agents.detail.tasksTitle': 'Task yang ditugaskan ke agent ini',
  'agents.detail.tasksEmpty': 'Riwayat task tersedia di masing-masing board.',
  'agents.detail.agent': 'agent',
  'agents.detail.save': 'Simpan perubahan',
  'agents.detail.saving': 'Menyimpan…',
  'agents.detail.saved': 'Perubahan tersimpan.',
  'agents.detail.saveFailed': 'Perubahan tidak tersimpan',
  'agents.detail.loading': 'Memuat agent…',
  'agents.detail.notFound': 'Agent tidak ditemukan',
  'agents.detail.archiveAction': 'Arsipkan agent',
  'agents.detail.unarchiveAction': 'Batal arsip agent',
  'agents.archive.forbidden': 'Hanya owner atau admin workspace yang dapat mengarsipkan atau membatalkan arsip agent.',
  'agents.archive.running': 'Agent ini masih punya task yang berjalan. Tunggu sampai selesai sebelum mengarsipkan.',
  'agents.archive.failed': 'Status arsip tidak berubah',
  'agents.detail.lifecycleTitle': 'Siklus penugasan',
  'agents.detail.lifecycleFooterActive': 'Berlaku: run dituntaskan · keluar dari picker assign',
  'agents.detail.lifecycleFooterArchived': 'Berlaku: run dituntaskan · disembunyikan dari penugasan baru',
  'agents.detail.assignmentState': 'Status penugasan',
  'agents.detail.rule1Title': 'Task berjalan tidak pernah diputus',
  'agents.detail.rule1Body': 'Task yang sedang berjalan tetap diselesaikan, dan biayanya tetap dicatat ke ledger.',
  'agents.detail.rule2Title': 'Keluar dari dropdown penugasan',
  'agents.detail.rule2Body':
    'Agent yang diarsip tidak dirender di dropdown penugasan pada Kanban board maupun table view. Barisnya tetap ada di sini supaya bisa dikembalikan.',
  'agents.detail.toolsGranted': 'Tools yang diberikan',
  'agents.detail.skillsInUse': 'Skill yang dipakai',
  'agents.detail.toolsHint': 'Tools adalah daftar tertutup — nama di luar daftar ditolak.',
  'agents.detail.skillsHint': 'Agent hanya membaca skill; tidak pernah menulisnya.',
  'agents.detail.gatePolicy': 'Gerbang persetujuan',
  'agents.detail.gateGated': 'require (gated)',
  'agents.detail.gateNone': 'tidak diberikan',
  'agents.detail.priceTitle': 'Estimasi tarif',
  'agents.detail.pricePriced': 'tabel harga',
  'agents.detail.priceUnpriced': 'belum ada harga',
  'agents.detail.priceInput': 'Masuk / 1 juta token',
  'agents.detail.priceOutput': 'Keluar / 1 juta token',
  'agents.detail.priceCached': 'Cache / 1 juta token',
  'agents.detail.priceVersion': 'tabel harga v{0}',
  'agents.detail.priceDisclaimer':
    'Tarif ini dari tabel harga internal AgentDeck, bukan tagihan provider. Biaya sebenarnya ada di dashboard masing-masing provider.',
  'agents.detail.priceLoading': 'memeriksa',
  'agents.detail.priceLoadingBody': 'Membaca tabel harga buat model ini…',
  'agents.detail.priceNoProvider': 'Agent ini nggak punya provider sendiri, jadi belum ada model yang dipatok.',
  'agents.detail.priceUnpricedBody': 'Belum ada entri harga buat {0}.',
  'agents.detail.priceNoModel': 'model yang dipilih',
  'agents.detail.providerHint':
    'Model di luar katalog tetap tersimpan apa adanya, tetapi tidak bisa dihitung biayanya.',
  'agents.detail.minutes': '({0} menit)',
  'agents.detail.notFoundHint': 'Mungkin sudah dihapus, atau milik ruang kerja lain.',
  'agents.detail.saveHint': 'Simpan mengirim seluruh profil ini; pilihan di atas hanya menentukan nilainya.',
  'agents.detail.skillsEmpty': 'Belum ada skill di ruang kerja ini',
  'agents.detail.justNow': 'baru saja',
  'agents.detail.minutesAgo': '{0} menit lalu',
  'agents.detail.hoursAgo': '{0} jam lalu',
  'agents.detail.daysAgo': '{0} hari lalu',
  'agents.detail.never': 'belum pernah',
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
