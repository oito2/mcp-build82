🌐 [Português](../../pt-br/getting-started/first-plugin.md) | **English** | 🏠 [Index](../index.md)

---

# Creating your First Plugin

In this guide, we will use the AI assistant to create a Moodle plugin from scratch, ensuring that the folder and file structure is 100% correct and aligned with Moodle standards.

---

## 1. The Challenge

We will create a **Local** type plugin called `my_tools` that will have:

- A database table to store simple logs.
- A scheduled task that runs daily.
- A specific capability for managers.

---

## 2. Generating the Structure (Scaffold)

`build82` exposes an **MCP prompt** called `scaffold_plugin` that automatically injects the full Moodle context — version, coding standards, and naming conventions read straight from `MOODLE_DEV_RULES.md` — into the scaffold generation. Invoke it in the AI chat:

```
scaffold_plugin
  type="local"
  name="my_tools"
  description="Utility tools with logs, scheduled task, and capability for managers"
  features="database tables, scheduled tasks, capabilities"
```

> In clients that expose MCP prompts as slash commands (e.g. Gemini Code Assist's Agent mode), it's also available as `/scaffold_plugin`.

> **Deterministic alternative:** the `scaffold_plugin` prompt only assembles instructions for the AI. To have build82 itself write the skeleton to disk (`version.php`, `lang/en/local_my_tools.php`, and one stub per feature), ask: _"Create the skeleton for a new local plugin called my_tools with database, tasks and capabilities."_ The AI will call the `create_plugin_skeleton` tool (`features="database,tasks,capabilities"`), which refuses to run if the plugin directory already exists. See the [Tools Reference](../reference/tools.md#create_plugin_skeleton).

The AI will generate the complete file structure following Moodle standards:

```
local/my_tools/
├── version.php
├── lang/
│   └── en/
│       └── local_my_tools.php
└── db/
    ├── install.xml
    ├── tasks.php
    └── access.php
```

---

## 3. Defining the Database

With the structure created, ask the AI to detail the logs table:

```
In the db/install.xml file of the local_my_tools plugin, create a table
called local_my_tools_logs with the fields: id, userid, action (string)
and timecreated.
```

> **Tip:** The AI will format the XML following the exact Moodle standard, including the attributes `NOTNULL`, `SEQUENCE`, and `NEXT` as required by XMLDB.

---

## 4. Generating Development Context

Now that the initial files have been created on disk, you need the AI to "learn" about this new plugin in order to help you with the programming logic.

**Run in the chat:**

```
Generate the AI context for the plugin local_my_tools.
```

This will call the tool `generate_plugin_context`, creating the 12 `PLUGIN_*.md` files under `local/my_tools/.build82/`. From now on, the AI will have full awareness that the `local_my_tools_logs` table exists and how it is structured. The plugin is also automatically marked `.indevelopment`, so it's picked up by `plugin_batch mode="dev"` and `update_indexes include_plugins=true` from now on.

Next, enable watch mode so the context updates automatically as you develop:

```
Start monitoring the plugin local_my_tools for file changes.
```

The AI will call `watch_plugins action="start"`. Any `db/*.php` or `version.php` file saved inside the plugin triggers an automatic background context update.

---

## 5. Implementing the Logic

Now ask the AI to write the real PHP code:

```
Create the Scheduled Task class in classes/task/log_cleanup_task.php.
It should delete records older than 30 days from the table local_my_tools_logs.
```

Since the AI has read the `PLUGIN_DB_TABLES.md` generated in the previous step, it will know exactly the table and field names to write the `$DB->delete_records_select()` query correctly.

---

## ✅ Completion Checklist

- [ ] Check if `version.php` has the correct `component`: `local_my_tools`.
- [ ] Install the plugin via the Moodle interface or via CLI: `php admin/cli/upgrade.php`.
- [ ] Ask your assistant: _"Run the build82 doctor"_ — the AI will call the `doctor` tool and confirm if the new plugin was indexed correctly.
- [ ] If you are actively developing, confirm that watch mode is active: _"What is the plugin monitoring status?"_

---

> **Next Step:** If you work with Docker, see how to fit build82 into that workflow in the **[Docker](../guides/environments/docker.md)** guide.
