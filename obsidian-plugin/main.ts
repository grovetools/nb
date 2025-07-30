import { App, Plugin, PluginSettingTab, Setting, ItemView, WorkspaceLeaf, Menu, TFile, Notice } from 'obsidian';
import { spawn } from 'child_process';
import * as path from 'path';
import * as fs from 'fs/promises';

interface NBNote {
  path: string;
  title: string;
  type: string;
  content: string;
  workspace: string;
  branch: string;
  created_at: string;
  modified_at: string;
  word_count: number;
  has_todos: boolean;
  is_archived: boolean;
  id?: string;
  aliases?: string[];
  tags?: string[];
  repository?: string;
}

interface NBSettings {
  nbExecutablePath: string;
  defaultRepository: string;
  defaultBranch: string;
}

const DEFAULT_SETTINGS: NBSettings = {
  nbExecutablePath: 'nb',
  defaultRepository: '',
  defaultBranch: 'main'
}

const VIEW_TYPE_NB_NOTES = "nb-notes-view";

export default class NBPlugin extends Plugin {
  settings: NBSettings;

  async onload() {
    await this.loadSettings();

    // Register the view
    this.registerView(
      VIEW_TYPE_NB_NOTES,
      (leaf) => new NBNotesView(leaf, this)
    );

    // Add ribbon icon
    this.addRibbonIcon('git-branch', 'NB Notes', () => {
      this.activateView();
    });

    // Add command to open view
    this.addCommand({
      id: 'open-nb-notes',
      name: 'Open NB Notes',
      callback: () => {
        this.activateView();
      }
    });

    // Add settings tab
    this.addSettingTab(new NBSettingTab(this.app, this));
  }

  async activateView() {
    const { workspace } = this.app;

    let leaf: WorkspaceLeaf | null = null;
    const leaves = workspace.getLeavesOfType(VIEW_TYPE_NB_NOTES);

    if (leaves.length > 0) {
      leaf = leaves[0];
    } else {
      leaf = workspace.getRightLeaf(false);
      if (leaf) {
        await leaf.setViewState({ type: VIEW_TYPE_NB_NOTES, active: true });
      }
    }

    if (leaf) {
      workspace.revealLeaf(leaf);
    }
  }

  async loadSettings() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
  }

  async saveSettings() {
    await this.saveData(this.settings);
  }

  async executeNBCommand(args: string[], cwd?: string): Promise<string> {
    return this.executeNBCommandInDir(args, cwd);
  }

  async executeNBCommandInDir(args: string[], cwd?: string): Promise<string> {
    return new Promise((resolve, reject) => {
      const nbPath = this.settings.nbExecutablePath;

      try {
        // When using shell: true, we need to pass the command as a single string
        const command = `${nbPath} ${args.join(' ')}`;
        const child = spawn(command, [], {
          env: { ...process.env },
          shell: true,
          cwd: cwd || process.cwd()
        });

        let stdout = '';
        let stderr = '';

        child.stdout.on('data', (data) => {
          stdout += data.toString();
        });

        child.stderr.on('data', (data) => {
          stderr += data.toString();
        });

        child.on('error', (error) => {
          reject(new Error(`Failed to spawn nb process: ${error.message}`));
        });

        child.on('close', (code) => {
          if (code === 0) {
            resolve(stdout);
          } else {
            reject(new Error(`NB command failed with code ${code}: ${stderr}`));
          }
        });
      } catch (error) {
        reject(new Error(`Failed to execute nb command: ${error.message}`));
      }
    });
  }
}

class NBNotesView extends ItemView {
  plugin: NBPlugin;
  repository: string;
  branch: string;
  notes: NBNote[] = [];

  constructor(leaf: WorkspaceLeaf, plugin: NBPlugin) {
    super(leaf);
    this.plugin = plugin;
    this.repository = plugin.settings.defaultRepository;
    this.branch = plugin.settings.defaultBranch;
  }

  getViewType() {
    return VIEW_TYPE_NB_NOTES;
  }

  getDisplayText() {
    return "NB Notes";
  }

  getIcon() {
    return "git-branch";
  }

  async onOpen() {
    const container = this.containerEl.children[1] as HTMLElement;
    container.empty();
    container.addClass('nb-notes-view');

    // Build the UI
    this.createHeader(container);
    this.createSearchBar(container);
    this.createTable(container);
    this.createFooter(container);

    // Attach behavior
    this.attachEventListeners();

    // Initial data load
    await this.loadNotes();
  }

  private createHeader(container: HTMLElement) {
    const header = container.createDiv('nb-header');
    this.createRepoSelector(header);
    this.createBranchSelector(header);
    this.createActionButtons(header);
  }

  private createRepoSelector(parent: HTMLElement) {
    const repoSelect = parent.createEl('select', { cls: 'nb-repo-select' });
    const repoOption = repoSelect.createEl('option', { text: this.repository || 'Select repository' });
  }

  private createBranchSelector(parent: HTMLElement) {
    const branchSelect = parent.createEl('select', { cls: 'nb-branch-select' });
    const branchOption = branchSelect.createEl('option', { text: this.branch });
  }

  private createActionButtons(parent: HTMLElement) {
    const actions = parent.createDiv('nb-actions');
    const loadButton = actions.createEl('button', { text: '📋 Load', cls: 'nb-action-button' });
    const newButton = actions.createEl('button', { text: '+ New', cls: 'nb-action-button' });
    const archiveButton = actions.createEl('button', { text: '⏳ Archive', cls: 'nb-action-button' });
    const syncButton = actions.createEl('button', { text: '↻ Sync', cls: 'nb-action-button' });
  }

  private createSearchBar(container: HTMLElement) {
    const searchContainer = container.createDiv('nb-search');
    const searchInput = searchContainer.createEl('input', {
      type: 'text',
      placeholder: '🔍 Search...',
      cls: 'nb-search-input'
    });
  }

  private createTable(container: HTMLElement) {
    const tableContainer = container.createDiv('nb-table-container');
    const table = tableContainer.createEl('table', { cls: 'nb-notes-table' });

    // Table header
    const thead = table.createEl('thead');
    const headerRow = thead.createEl('tr');
    headerRow.createEl('th', { text: 'TYPE' });
    headerRow.createEl('th', { text: 'TITLE' });
    headerRow.createEl('th', { text: 'TAGS' });
    headerRow.createEl('th', { text: 'TIME' });

    // Table body
    const tbody = table.createEl('tbody');
    tbody.setAttribute('id', 'nb-notes-tbody');
  }

  private createFooter(container: HTMLElement) {
    // Note count
    const countContainer = container.createDiv('nb-count-container');
    countContainer.createEl('div', { cls: 'nb-count', text: 'Loading notes...' });

    // Quick note input
    const quickNoteContainer = container.createDiv('nb-quick-note');
    const quickNoteInput = quickNoteContainer.createEl('input', {
      type: 'text',
      placeholder: '💡 Quick note... [Cmd+Enter]',
      cls: 'nb-quick-note-input'
    });
  }

  private attachEventListeners() {
    // Load button
    this.containerEl.querySelector('.nb-action-button:nth-child(1)')?.addEventListener('click', async () => {
      const countEl = this.containerEl.querySelector('.nb-count');
      if (countEl) {
        countEl.textContent = 'Loading notes...';
      }
      await this.loadNotes();
    });

    // New button
    this.containerEl.querySelector('.nb-action-button:nth-child(2)')?.addEventListener('click', () => this.createNewNote());

    // Archive button
    this.containerEl.querySelector('.nb-action-button:nth-child(3)')?.addEventListener('click', () => this.archiveCurrentNote());

    // Sync button
    this.containerEl.querySelector('.nb-action-button:nth-child(4)')?.addEventListener('click', () => this.syncRepository());

    // Search input
    this.containerEl.querySelector('.nb-search-input')?.addEventListener('input', (e) => {
      this.filterNotes((e.target as HTMLInputElement).value);
    });

    // Quick note input
    this.containerEl.querySelector('.nb-quick-note-input')?.addEventListener('keydown', (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        this.createQuickNote((e.target as HTMLInputElement).value);
        (e.target as HTMLInputElement).value = '';
      }
    });
  }

  async loadNotes() {
    try {
      // Get all notes from all workspaces using the new flag
      const output = await this.plugin.executeNBCommand(['list', '--workspaces', '--json']);
      this.notes = this.parseNBOutput(output);

      // Extract unique workspaces from notes
      const workspaceSet = new Set(this.notes.map(n => n.workspace));
      const workspaces = Array.from(workspaceSet).filter(w => w); // Remove empty strings

      // Update repository dropdown
      if (workspaces.length > 0) {
        this.updateRepositoryDropdown(workspaces);

        // Set default repository if not set
        if (!this.repository && workspaces.length > 0) {
          this.repository = workspaces[0];
        }
      }

      // Extract branches for current workspace
      const currentWorkspaceNotes = this.notes.filter(n => n.workspace === this.repository);
      const branchSet = new Set(currentWorkspaceNotes.map(n => n.branch).filter(b => b));
      const branches = Array.from(branchSet);

      // Global workspace doesn't have branches
      if (this.repository === 'global') {
        this.branch = '';
        this.updateBranchDropdown([]);
      } else {
        if (branches.length === 0) {
          branches.push('main'); // Default
        }
        this.updateBranchDropdown(branches);
      }
      this.renderNotes();
    } catch (error) {
      console.error('Failed to load notes:', error);
      new Notice('Failed to load notes: ' + error.message);
    }
  }



  parseNBOutput(output: string): NBNote[] {
    try {
      const notes = JSON.parse(output);
      // The Go backend now provides all metadata including tags
      return notes as NBNote[];
    } catch (error) {
      console.error('Failed to parse nb output:', error);
      return [];
    }
  }

  renderNotes() {
    const tbody = this.containerEl.querySelector('#nb-notes-tbody');
    if (!tbody) return;

    tbody.empty();

    // Filter notes for current repository and branch
    const filteredNotes = this.notes.filter(note => {
      // For global workspace, only check workspace
      if (this.repository === 'global') {
        return note.workspace === 'global';
      }
      // For other workspaces, check both workspace and branch
      return note.workspace === this.repository &&
        note.branch === this.branch;
    });

    filteredNotes.forEach(note => {
      const row = tbody.createEl('tr');
      row.createEl('td', { text: note.type });
      row.createEl('td', { text: note.title });
      row.createEl('td', { text: note.tags?.join(', ') || '-' });

      // Format time ago
      const modified = new Date(note.modified_at);
      const timeAgo = this.formatTimeAgo(modified);
      row.createEl('td', { text: timeAgo });

      row.addEventListener('click', () => this.openNote(note));
    });

    // Update count
    const countEl = this.containerEl.querySelector('.nb-count');
    if (countEl) {
      countEl.textContent = `Showing ${filteredNotes.length} notes`;
    }
  }

  formatTimeAgo(date: Date): string {
    const seconds = Math.floor((new Date().getTime() - date.getTime()) / 1000);

    let interval = seconds / 31536000;
    if (interval > 1) return Math.floor(interval) + "y";

    interval = seconds / 2592000;
    if (interval > 1) return Math.floor(interval) + "mo";

    interval = seconds / 86400;
    if (interval > 1) return Math.floor(interval) + "d";

    interval = seconds / 3600;
    if (interval > 1) return Math.floor(interval) + "h";

    interval = seconds / 60;
    if (interval > 1) return Math.floor(interval) + "m";

    return Math.floor(seconds) + "s";
  }

  updateRepositoryDropdown(repositories: string[]) {
    const repoSelect = this.containerEl.querySelector('.nb-repo-select') as HTMLSelectElement;
    if (!repoSelect) return;

    repoSelect.empty();

    if (!this.repository && repositories.length > 0) {
      this.repository = repositories[0];
    }

    repositories.forEach(repo => {
      const option = repoSelect.createEl('option', {
        text: repo,
        value: repo
      });
      if (repo === this.repository) {
        option.selected = true;
      }
    });

    repoSelect.addEventListener('change', (e) => {
      this.repository = (e.target as HTMLSelectElement).value;
      this.loadNotes();
    });
  }

  updateBranchDropdown(branches: string[]) {
    const branchSelect = this.containerEl.querySelector('.nb-branch-select') as HTMLSelectElement;
    if (!branchSelect) return;

    branchSelect.empty();

    // Hide branch selector for workspaces without branches
    if (branches.length === 0) {
      branchSelect.style.display = 'none';
      return;
    } else {
      branchSelect.style.display = '';
    }

    if (!this.branch && branches.length > 0) {
      this.branch = branches[0];
    }

    branches.forEach(branch => {
      const option = branchSelect.createEl('option', {
        text: branch,
        value: branch
      });
      if (branch === this.branch) {
        option.selected = true;
      }
    });

    branchSelect.addEventListener('change', (e) => {
      this.branch = (e.target as HTMLSelectElement).value;
      this.renderNotes();
    });
  }

  filterNotes(query: string) {
    const tbody = this.containerEl.querySelector('#nb-notes-tbody');
    if (!tbody) return;

    const rows = tbody.querySelectorAll('tr');
    const lowerQuery = query.toLowerCase();

    rows.forEach((row: HTMLTableRowElement) => {
      const text = row.textContent?.toLowerCase() || '';
      row.style.display = text.includes(lowerQuery) ? '' : 'none';
    });
  }

  async createNewNote() {
    try {
      await this.plugin.executeNBCommand(['new']);
      await this.loadNotes();
      new Notice('Created new note');
    } catch (error) {
      new Notice('Failed to create note: ' + error.message);
    }
  }

  async archiveCurrentNote() {
    const activeFile = this.app.workspace.getActiveFile();
    if (!activeFile) {
      new Notice('No active file to archive');
      return;
    }

    // Find the corresponding nb note
    const vaultPath = (this.app.vault.adapter as any).basePath || '';
    const fullPath = path.join(vaultPath, activeFile.path);

    const activeNote = this.notes.find(note => note.path === fullPath);

    if (!activeNote) {
      new Notice('This is not an nb note');
      return;
    }

    try {
      // Extract just the filename from the full path
      const fileName = path.basename(activeNote.path);

      // Execute archive command with workspace override and force flag to skip confirmation
      await this.plugin.executeNBCommand(
        ['archive', fileName, '--workspace', activeNote.workspace, '--force']
      );

      // Reload notes to refresh the list
      await this.loadNotes();

      new Notice(`Archived ${activeNote.title}`);
    } catch (error) {
      new Notice('Failed to archive note: ' + error.message);
    }
  }

  async syncRepository() {
    try {
      new Notice('Syncing repository...');
      // Run git pull/push commands through nb
      await this.plugin.executeNBCommand(['workspace', 'sync']);
      await this.loadNotes();
      new Notice('Repository synced');
    } catch (error) {
      new Notice('Failed to sync: ' + error.message);
    }
  }

  async createQuickNote(content: string) {
    if (!content.trim()) return;

    try {
      await this.plugin.executeNBCommand(['quick', content]);
      await this.loadNotes();
      new Notice('Created quick note');
    } catch (error) {
      new Notice('Failed to create quick note: ' + error.message);
    }
  }

  async openNote(note: NBNote) {
    // nb notes should already be in the vault
    const vaultPath = (this.app.vault.adapter as any).basePath || '';

    // Convert absolute path to relative path within vault
    let relativePath: string;
    if (note.path.startsWith(vaultPath)) {
      relativePath = note.path.substring(vaultPath.length + 1);
    } else {
      new Notice('Note is outside the vault: ' + note.path);
      return;
    }

    // Get the file from vault
    const file = this.app.vault.getAbstractFileByPath(relativePath);
    if (file instanceof TFile) {
      await this.app.workspace.getLeaf().openFile(file);
    } else {
      new Notice('Note file not found in vault: ' + relativePath);
    }
  }

  async onClose() {
    // Clean up
  }
}

class NBSettingTab extends PluginSettingTab {
  plugin: NBPlugin;

  constructor(app: App, plugin: NBPlugin) {
    super(app, plugin);
    this.plugin = plugin;
  }

  display(): void {
    const { containerEl } = this;

    containerEl.empty();

    new Setting(containerEl)
      .setName('NB executable path')
      .setDesc('Path to the nb command line tool')
      .addText(text => text
        .setPlaceholder('nb')
        .setValue(this.plugin.settings.nbExecutablePath)
        .onChange(async (value) => {
          this.plugin.settings.nbExecutablePath = value;
          await this.plugin.saveSettings();
        }));

    new Setting(containerEl)
      .setName('Default repository')
      .setDesc('Default repository to display')
      .addText(text => text
        .setPlaceholder('note-system')
        .setValue(this.plugin.settings.defaultRepository)
        .onChange(async (value) => {
          this.plugin.settings.defaultRepository = value;
          await this.plugin.saveSettings();
        }));

    new Setting(containerEl)
      .setName('Default branch')
      .setDesc('Default branch to display')
      .addText(text => text
        .setPlaceholder('main')
        .setValue(this.plugin.settings.defaultBranch)
        .onChange(async (value) => {
          this.plugin.settings.defaultBranch = value;
          await this.plugin.saveSettings();
        }));
  }
}
