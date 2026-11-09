import "./style.css";
import { App } from "../bindings/github.com/younes-alkashmi/todo";

type State = {
  projects: ProjectDTO[];
  tasks: TaskDTO[];
  selectedId: string | null;
  scopeProjectId: string | null;
  error: { field: string; message: string } | null;
  flash: string | null;
  view: "list" | "details" | "sidebar";
  draft: {
    taskId: string;
    title: string;
    description: string;
    dueDate: string;
    tags: string;
  } | null;
};

const state: State = {
  projects: [],
  tasks: [],
  selectedId: null,
  scopeProjectId: null,
  error: null,
  flash: null,
  view: "list",
  draft: null,
};

// --- helpers ---
const $ = <T extends HTMLElement = HTMLElement>(sel: string): T =>
  document.querySelector(sel) as T;

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K, attrs: Record<string, any> = {}, ...children: (Node | string)[]
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "onclick") node.addEventListener("click", v);
    else if (k === "class") node.className = v;
    else node.setAttribute(k, String(v));
  }
  for (const c of children) node.append(c);
  return node;
}

// Flash with a token so re-renders don't stack timeouts and clear each other's work.
let flashToken = 0;
function setFlash(msg: string) {
  state.flash = msg;
  const token = ++flashToken;
  renderFlash();
  setTimeout(() => {
    if (token === flashToken) {
      state.flash = null;
      renderFlash();
    }
  }, 2000);
}

// Whenever we change the selected task, drop any pending draft. Otherwise
// a user's unsaved edits on task A bleed into task B's form.
function selectTask(id: string | null, view: State["view"] = "details") {
  state.selectedId = id;
  state.draft = null;
  state.view = view;
  render();
}

// --- data loading ---
async function refresh() {
  state.projects = await App.ListProjects();
  state.tasks = await App.ListTasks(state.scopeProjectId);
}

// --- render ---
function render() {
  const appEl = document.getElementById("app");
  if (appEl) appEl.setAttribute("data-view", state.view);

  renderSidebar();
  renderList();
  renderDetails();
  renderFlash();
}

function renderSidebar() {
  const root = $("#sidebar");
  root.innerHTML = "";

  root.append(el("h1", {}, "Todo"));

  const globalBtn = el("button", {
    class: state.scopeProjectId === "" ? "nav active" : "nav",
    onclick: () => {
      state.scopeProjectId = "";
      state.selectedId = null;
      state.draft = null;
      state.view = "list";
      refreshAndRender();
    },
  }, "Global tasks");
  root.append(globalBtn);

  root.append(el("h2", {}, "Projects"));
  for (const p of state.projects) {
    root.append(el("button", {
      class: state.scopeProjectId === p.id ? "nav active" : "nav",
      onclick: () => {
        state.scopeProjectId = p.id;
        state.selectedId = null;
        state.draft = null;
        state.view = "list";
        refreshAndRender();
      },
    }, p.name));
  }
  root.append(el("button", {
    class: "nav add",
    onclick: async () => {
      const name = prompt("Project name");
      if (!name) return;
      try {
        await App.CreateProject(name);
        await refreshAndRender();
      } catch (e: any) { setFlash(e.message ?? String(e)); }
    },
  }, "+ New project"));
}

function renderList() {
  const root = $("#list");
  root.innerHTML = "";

  const scopeName =
    state.scopeProjectId === null ? "All" :
    state.scopeProjectId === "" ? "Global tasks" :
    (state.projects.find(p => p.id === state.scopeProjectId)?.name ?? "Project");

  root.append(el("h2", {}, scopeName));

  const addInput = el("input", {
    placeholder: "+ Add task (press Enter)",
    class: "add-input",
  }) as HTMLInputElement;
  addInput.addEventListener("keydown", async (ev) => {
    if (ev.key !== "Enter") return;
    const title = addInput.value;
    try {
      const created = await App.CreateTask({
        title,
        description: "",
        projectId: state.scopeProjectId || null,
        parentId: null,
        dueDate: null,
        tags: [],
      } as any);
      state.selectedId = created.id;
      state.draft = null;
      setFlash("Saved locally");
      await refreshAndRender();
    } catch (e: any) {
      setFlash(e.message ?? String(e));
    }
  });
  root.append(addInput);

  const roots = state.tasks.filter(t => t.parentId === null);
  const childrenOf = (id: string) => state.tasks.filter(t => t.parentId === id);

  for (const t of roots) {
    root.append(taskRow(t, false));
    for (const c of childrenOf(t.id)) {
      root.append(taskRow(c, true));
    }
  }

  if (roots.length === 0) {
    root.append(el("p", { class: "empty" }, "No tasks yet."));
  }
}

function taskRow(t: TaskDTO, isSub: boolean) {
  const row = el("div", {
    class: (state.selectedId === t.id ? "row selected" : "row") + (isSub ? " sub" : ""),
    onclick: () => selectTask(t.id),
  });

  const cb = el("input", { type: "checkbox" }) as HTMLInputElement;
  cb.checked = t.completed;
  cb.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    try {
      await App.SetTaskCompleted(t.id, cb.checked);
      await refreshAndRender();
    } catch (e: any) { setFlash(e.message); }
  });
  row.append(cb);

  row.append(el("span", { class: t.completed ? "title done" : "title" }, t.title));
  if (t.dueDate) row.append(el("span", { class: "meta" }, t.dueDate));
  if (t.tags.length) row.append(el("span", { class: "meta" }, t.tags.join(", ")));

  return row;
}

function renderDetails() {
  const root = $("#details");
  root.innerHTML = "";

  const t = state.tasks.find(x => x.id === state.selectedId);
  if (!t) {
    root.append(el("p", { class: "empty" }, "Select a task."));
    return;
  }

  // If there is no draft yet, or the draft belongs to a different task,
  // seed it from the persisted model.
  if (!state.draft || state.draft.taskId !== t.id) {
    state.draft = {
      taskId: t.id,
      title: t.title,
      description: t.description,
      dueDate: t.dueDate ?? "",
      tags: t.tags.join(", "),
    };
  }
  const draft = state.draft;

  root.append(el("h2", {}, "Task details"));

  const titleInput = el("input", { class: "field", value: draft.title }) as HTMLInputElement;
  titleInput.addEventListener("input", () => { draft.title = titleInput.value; });
  root.append(field("Title", titleInput));

  const desc = el("textarea", { class: "field", rows: 4 }) as HTMLTextAreaElement;
  desc.value = draft.description;
  desc.addEventListener("input", () => { draft.description = desc.value; });
  root.append(field("Description", desc));

  const due = el("input", { class: "field", type: "date", value: draft.dueDate }) as HTMLInputElement;
  due.addEventListener("input", () => { draft.dueDate = due.value; });
  root.append(field("Due date", due));

  const tags = el("input", { class: "field", value: draft.tags }) as HTMLInputElement;
  tags.addEventListener("input", () => { draft.tags = tags.value; });
  root.append(field("Tags", tags));

  if (state.error) {
    root.append(el("p", { class: "error" }, state.error.message));
  }

  const saveBtn = el("button", { class: "primary" }, "Save changes");
  saveBtn.addEventListener("click", async () => {
    state.error = null;
    try {
      const updated = await App.UpdateTask(t.id, {
        title: draft.title,
        description: draft.description,
        projectId: t.projectId,
        parentId: t.parentId,
        dueDate: draft.dueDate || null,
        tags: draft.tags.split(",").map(s => s.trim()).filter(Boolean),
      } as any);
      state.draft = null;
      state.selectedId = updated.id;
      setFlash("Saved locally");
      await refreshAndRender();
    } catch (e: any) {
      state.error = { field: "title", message: e.message ?? String(e) };
      render();
    }
  });
  root.append(saveBtn);

  const delBtn = el("button", { class: "danger" }, "Delete task");
  delBtn.addEventListener("click", async () => {
    const childCount = state.tasks.filter(x => x.parentId === t.id).length;
    const msg = childCount
      ? `Delete this task and ${childCount} subtask(s)?`
      : "Delete this task?";
    if (!confirm(msg)) return;
    try {
      await App.DeleteTask(t.id);
      state.selectedId = null;
      state.draft = null;
      state.view = "list";
      setFlash("Deleted");
      await refreshAndRender();
    } catch (e: any) { setFlash(e.message); }
  });
  root.append(delBtn);

  root.append(el("h3", {}, "Subtasks"));
  if (!t.parentId) {
    const subInput = el("input", { placeholder: "+ Add subtask (Enter)", class: "add-input" }) as HTMLInputElement;
    subInput.addEventListener("keydown", async (ev) => {
      if (ev.key !== "Enter") return;
      try {
        await App.CreateTask({
          title: subInput.value,
          description: "",
          projectId: t.projectId,
          parentId: t.id,
          dueDate: null,
          tags: [],
        } as any);
        setFlash("Saved locally");
        await refreshAndRender();
      } catch (e: any) { setFlash(e.message); }
    });
    root.append(subInput);
  } else {
    root.append(el("p", { class: "empty" }, "Subtasks cannot have subtasks."));
  }

  const subs = state.tasks.filter(x => x.parentId === t.id);
  for (const s of subs) root.append(taskRow(s, true));
}

function field(label: string, input: HTMLElement): HTMLElement {
  return el("div", { class: "field-group" }, el("label", {}, label), input);
}

function renderFlash() {
  const el_ = $("#flash");
  el_.textContent = state.flash ?? "Offline · Local only";
}

async function refreshAndRender() {
  try {
    await refresh();
  } catch (e: any) {
    setFlash(e.message ?? String(e));
  }
  render();
}

function isNarrow(): boolean {
  return window.matchMedia("(max-width: 720px)").matches;
}

function go(next: State["view"]) {
  state.view = next;
  render();
}

window.matchMedia("(max-width: 720px)").addEventListener("change", () => {
  if (!isNarrow()) state.view = "list";
  render();
});

// Kick off
refreshAndRender(); 