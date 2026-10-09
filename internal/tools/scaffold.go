// Copyright (C) 2026  OITO2
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// skeletonNamePattern matches valid plugin names: lowercase, starting with a letter, with only
// letters, digits and underscores. Because the name becomes a directory name, this also prevents
// path traversal (e.g. "../../etc").
var skeletonNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// requiresPattern matches the accepted $plugin->requires values: digits with an optional single
// decimal point, i.e. a plain PHP numeric literal. The value is written unquoted into version.php
// (see versionPhpSkeleton), so anything else could inject PHP code.
var requiresPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// CreatePluginSkeletonInput is the input of the create_plugin_skeleton tool.
type CreatePluginSkeletonInput struct {
	Type        string `json:"type" jsonschema:"Plugin type (local, mod, block, auth, tool, enrol, theme, report, format, filter, qtype, ...)"`
	Name        string `json:"name" jsonschema:"Plugin name, lowercase letters/digits/underscores only, must start with a letter"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Human-readable name for lang/en/{component}.php (default: derived from name)"`
	Features    string `json:"features,omitempty" jsonschema:"Comma-separated stub files to scaffold: database, tasks, services, events, capabilities, settings"`
	Requires    string `json:"requires,omitempty" jsonschema:"$plugin->requires value (default: the configured Moodle installation's build number)"`
	Maturity    string `json:"maturity,omitempty" jsonschema:"$plugin->maturity constant: MATURITY_ALPHA (default), MATURITY_BETA, MATURITY_RC, or MATURITY_STABLE"`
}

// RegisterCreatePluginSkeletonTool registers the create_plugin_skeleton tool on `server`.
func RegisterCreatePluginSkeletonTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_plugin_skeleton",
		Annotations: toolAnnotations("Create Plugin Skeleton", false, false, false, false),
		Description: "Materializes a new Moodle plugin's directory structure on disk: version.php with " +
			"real values filled in, lang/en/{component}.php, the type's mandatory entry-point file(s), and " +
			"stub db/*.php files for any requested features. Deterministic scaffolding only — no generated " +
			"business logic. Pair with the scaffold_plugin prompt for AI-drafted implementation content. " +
			"Refuses to run if the target plugin directory already exists.",
	}, withRecover(handleCreatePluginSkeleton))
}

// skeletonFile is one file to scaffold: its path and its content.
type skeletonFile struct {
	path    string // relative to the plugin root, forward slashes
	content string
}

// handleCreatePluginSkeleton creates the directory and stub files of a new plugin of type
// `in.Type` named `in.Name` under the Moodle root, with the optional stubs selected by
// `in.Features`. It never overwrites an existing plugin. If writing a file fails, everything this
// call created is removed. It returns an error result for a missing configuration, unknown type,
// invalid name, maturity or requires value, an existing target, or a write failure; the error
// return is always nil.
func handleCreatePluginSkeleton(ctx context.Context, req *mcp.CallToolRequest, in CreatePluginSkeletonInput) (*mcp.CallToolResult, any, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), nil, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), nil, nil
	}

	typeDir, ok := moodletype.PluginTypeToDir[in.Type]
	if !ok {
		return textResult(true, fmt.Sprintf("❌ Unknown plugin type %q.", in.Type)), nil, nil
	}
	if !skeletonNamePattern.MatchString(in.Name) {
		return textResult(true, "❌ name must start with a lowercase letter and contain only lowercase letters, digits, and underscores."), nil, nil
	}

	pluginPath := filepath.Clean(filepath.Join(cfg.MoodlePath, typeDir, in.Name))
	if !moodletype.IsWithinMoodle(pluginPath, cfg.MoodlePath) {
		return textResult(true, "❌ Resolved plugin path escapes the Moodle root."), nil, nil
	}
	if dirExists(pluginPath) || fileExists(pluginPath) {
		return textResult(true, fmt.Sprintf("❌ %s already exists — create_plugin_skeleton never overwrites an existing plugin.", relativeToMoodle(cfg.MoodlePath, pluginPath))), nil, nil
	}

	maturity := in.Maturity
	if maturity == "" {
		maturity = "MATURITY_ALPHA"
	} else if _, ok := validMoodleMaturities[maturity]; !ok {
		return textResult(true, fmt.Sprintf("❌ Unrecognized maturity %q (expected MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC, or MATURITY_STABLE).", maturity)), nil, nil
	}

	displayName := in.DisplayName
	if displayName == "" {
		displayName = strings.ReplaceAll(in.Name, "_", " ")
	}

	requires, requiresComment := in.Requires, ""
	if requires == "" {
		requires = cfg.MoodleFullVersion
	}
	if requires == "" {
		requires = "0"
		requiresComment = " // TODO: set to a real Moodle build number"
	} else if !requiresPattern.MatchString(requires) {
		// requires is written unquoted into version.php, so only a numeric literal is accepted;
		// anything else could inject PHP code (e.g. "0; eval(...);").
		return textResult(true, fmt.Sprintf(
			"❌ requires must be a plain Moodle build number (digits, optionally with a decimal point), got %q.", requires,
		)), nil, nil
	}

	component := in.Type + "_" + in.Name
	hasDB, hasTasks, hasServices, hasEvents, hasCaps, hasSettings := parseSkeletonFeatures(in.Features)

	files := skeletonFiles(skeletonParams{
		pluginType: in.Type, name: in.Name, component: component, typeDir: typeDir,
		displayName: displayName, requires: requires, requiresComment: requiresComment, maturity: maturity,
		hasDB: hasDB, hasTasks: hasTasks, hasServices: hasServices, hasEvents: hasEvents,
		hasCaps: hasCaps, hasSettings: hasSettings,
	})

	// createdRoot is the outermost directory this call will create: the plugin directory itself,
	// or a missing type directory above it. Everything under it is new, so removing it on failure
	// undoes exactly this call's writes.
	createdRoot := outermostMissingDir(cfg.MoodlePath, pluginPath)

	var written []string
	for _, f := range files {
		full := filepath.Join(pluginPath, filepath.FromSlash(f.path))
		if err := writeSkeletonFile(full, []byte(f.content), 0o644); err != nil {
			msg := fmt.Sprintf("❌ Failed to write %s: %v", f.path, err)
			if rmErr := os.RemoveAll(createdRoot); rmErr != nil {
				msg += fmt.Sprintf("\n\nCleanup of the partially created %s also failed: %v",
					relativeToMoodle(cfg.MoodlePath, createdRoot), rmErr)
			}
			return textResult(true, msg), nil, nil
		}
		written = append(written, f.path)
	}
	sort.Strings(written)

	var b strings.Builder
	fmt.Fprintf(&b, "✅ Created skeleton for %s at %s\n\n", component, relativeToMoodle(cfg.MoodlePath, pluginPath))
	b.WriteString("Files written:\n")
	for _, f := range written {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\nDeterministic skeleton only — no business logic was generated. Use the " +
		"`scaffold_plugin` prompt, or write the implementation directly, to fill it in.\n")
	return textResult(false, b.String()), nil, nil
}

// writeSkeletonFile writes one scaffolded file atomically. It defaults to fsutil.WriteAtomic and
// is a variable so tests can inject a write failure.
var writeSkeletonFile = fsutil.WriteAtomic

// outermostMissingDir returns the highest directory between `moodlePath` (exclusive) and `dir`
// (inclusive) that does not exist yet, i.e. the one directory whose removal undoes everything a
// later MkdirAll(dir) and writes under it create. `dir` itself must not exist.
func outermostMissingDir(moodlePath, dir string) string {
	root := filepath.Clean(moodlePath)
	outermost := dir
	for p := filepath.Dir(dir); p != root && p != filepath.Dir(p); p = filepath.Dir(p) {
		if _, err := os.Lstat(p); err == nil {
			break
		}
		outermost = p
	}
	return outermost
}

// parseSkeletonFeatures parses the comma-separated `features` list, case-insensitively and by
// keyword (e.g. "table" selects the database), into flags selecting the database, tasks, services,
// events, capabilities and settings stubs. Unrecognized entries are ignored.
func parseSkeletonFeatures(features string) (hasDB, hasTasks, hasServices, hasEvents, hasCaps, hasSettings bool) {
	for _, f := range strings.Split(strings.ToLower(features), ",") {
		f = strings.TrimSpace(f)
		switch {
		case strings.Contains(f, "database") || strings.Contains(f, "table"):
			hasDB = true
		case strings.Contains(f, "task"):
			hasTasks = true
		case strings.Contains(f, "service") || strings.Contains(f, "api"):
			hasServices = true
		case strings.Contains(f, "event"):
			hasEvents = true
		case strings.Contains(f, "capabilit") || strings.Contains(f, "permission"):
			hasCaps = true
		case strings.Contains(f, "setting"):
			hasSettings = true
		}
	}
	return
}

// skeletonParams holds the values and feature flags used to render the skeleton files.
type skeletonParams struct {
	pluginType, name, component, typeDir    string
	displayName, requires, requiresComment  string
	maturity                                string
	hasDB, hasTasks, hasServices, hasEvents bool
	hasCaps, hasSettings                    bool
}

// skeletonFiles returns every file to scaffold for `p`: version.php, the English language file,
// the type's entry-point files and the stubs selected by the feature flags. The version is today's
// date followed by "00".
func skeletonFiles(p skeletonParams) []skeletonFile {
	version := time.Now().Format("20060102") + "00"

	files := []skeletonFile{
		{"version.php", versionPhpSkeleton(p, version)},
		{"lang/en/" + p.component + ".php", langFileSkeleton(p.displayName)},
	}
	files = append(files, mainFilesSkeleton(p)...)

	if p.hasDB {
		files = append(files, skeletonFile{"db/install.xml", installXMLSkeleton(p)})
	}
	if p.hasTasks {
		files = append(files, skeletonFile{"db/tasks.php", tasksPhpSkeleton(p.component)})
	}
	if p.hasServices {
		files = append(files, skeletonFile{"db/services.php", servicesPhpSkeleton()})
	}
	if p.hasEvents {
		files = append(files, skeletonFile{"db/events.php", eventsPhpSkeleton(p.component)})
		files = append(files, skeletonFile{"classes/observer.php", observerPhpSkeleton(p.component)})
	}
	if p.hasCaps {
		files = append(files, skeletonFile{"db/access.php", accessPhpSkeleton(p.component)})
	}
	if p.hasSettings {
		files = append(files, skeletonFile{"settings.php", settingsPhpSkeleton(p.component)})
	}
	return files
}

// versionPhpSkeleton renders version.php for `p` with the given `version` number.
func versionPhpSkeleton(p skeletonParams, version string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$plugin->component = '%s';
$plugin->version   = %s;
$plugin->requires  = %s;%s
$plugin->maturity  = %s;
$plugin->release   = '1.0.0';
`, p.component, version, p.requires, p.requiresComment, p.maturity)
}

// langFileSkeleton renders the English language file declaring the plugin name `displayName`.
func langFileSkeleton(displayName string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$string['pluginname'] = '%s';
`, escapePhpSingleQuoted(displayName))
}

// escapePhpSingleQuoted escapes `s` for use inside a PHP single-quoted string literal by escaping
// backslashes and then single quotes (in that order, so the escapes are not doubled). This keeps
// free-text values from breaking out of the literal and injecting PHP code.
func escapePhpSingleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// mainFilesSkeleton returns the mandatory entry-point files for the plugin type of `p`: block,
// auth, mod and tool have their own, other types get a lib.php.
func mainFilesSkeleton(p skeletonParams) []skeletonFile {
	switch p.pluginType {
	case "block":
		return []skeletonFile{{"block_" + p.name + ".php", blockPhpSkeleton(p)}}
	case "auth":
		return []skeletonFile{{"auth.php", authPhpSkeleton(p)}}
	case "mod":
		return []skeletonFile{
			{"lib.php", modLibPhpSkeleton(p)},
			{"index.php", stubEntryPointSkeleton(p, "list all "+p.component+" instances in a course")},
			{"view.php", stubEntryPointSkeleton(p, "view a single "+p.component+" instance")},
			{"mod_form.php", modFormPhpSkeleton(p)},
		}
	case "tool":
		return []skeletonFile{{"index.php", stubEntryPointSkeleton(p, p.component+"'s admin tool page")}}
	default:
		return []skeletonFile{{"lib.php", libPhpSkeleton(p.component)}}
	}
}

// libPhpSkeleton renders a lib.php stub for `component`.
func libPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

// TODO: implement %s.
`, component)
}

// blockPhpSkeleton renders the block class stub for `p`.
func blockPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

class block_%s extends block_base {
    public function init() {
        $this->title = get_string('pluginname', '%s');
    }

    public function get_content() {
        if ($this->content !== null) {
            return $this->content;
        }
        $this->content = new stdClass();
        $this->content->text = ''; // TODO: implement.
        $this->content->footer = '';
        return $this->content;
    }
}
`, p.name, p.component)
}

// authPhpSkeleton renders the authentication plugin class stub for `p`.
func authPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

class auth_plugin_%s extends auth_plugin_base {
    public function __construct() {
        $this->authtype = '%s';
        $this->config = get_config('auth_%s');
    }

    public function user_login($username, $password) {
        // TODO: implement.
        return false;
    }
}
`, p.name, p.name, p.name)
}

// modLibPhpSkeleton renders the lib.php stub of an activity module with its add, update and
// delete instance callbacks.
func modLibPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

function %s_add_instance($data, $mform = null) {
    global $DB;
    // TODO: implement.
    return $DB->insert_record('%s', $data);
}

function %s_update_instance($data, $mform = null) {
    global $DB;
    $data->id = $data->instance;
    // TODO: implement.
    return $DB->update_record('%s', $data);
}

function %s_delete_instance($id) {
    global $DB;
    // TODO: implement.
    return $DB->delete_records('%s', array('id' => $id));
}
`, p.component, p.name, p.component, p.name, p.component, p.name)
}

// modFormPhpSkeleton renders the mod_form.php stub of an activity module.
func modFormPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

require_once($CFG->dirroot . '/course/moodleform_mod.php');

class mod_%s_mod_form extends moodleform_mod {
    public function definition() {
        $mform = $this->_form;

        $mform->addElement('text', 'name', get_string('pluginname', '%s'));
        $mform->setType('name', PARAM_TEXT);
        $mform->addRule('name', null, 'required', null, 'client');

        $this->standard_coursemodule_elements();
        $this->add_action_buttons();
    }
}
`, p.name, p.component)
}

// stubEntryPointSkeleton renders a web entry-point stub that loads config.php, with a TODO naming
// the page's `purpose`.
func stubEntryPointSkeleton(p skeletonParams, purpose string) string {
	return fmt.Sprintf(`<?php
require_once(__DIR__ . '/%sconfig.php');

// TODO: %s.
`, strings.Repeat("../", configDepth(p.typeDir)), purpose)
}

// configDepth returns the number of "../" segments an entry-point file needs to reach config.php
// at the Moodle root from {typeDir}/{name}/: the number of segments of `typeDir` plus one for the
// plugin directory, e.g. 2 for "mod" and 3 for "admin/tool".
func configDepth(typeDir string) int {
	return strings.Count(typeDir, "/") + 2
}

// installXMLSkeleton renders an empty db/install.xml for `p`.
func installXMLSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
<XMLDB PATH="%s/%s/db" VERSION="%s" COMMENT="XMLDB file for Moodle %s"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xsi:noNamespaceSchemaLocation="%slib/xmldb/xmldb.xsd"
>
  <TABLES>
    <!-- TODO: define tables here. -->
  </TABLES>
</XMLDB>
`, p.typeDir, p.name, time.Now().Format("20060102")+"00", p.component, strings.Repeat("../", configDepth(p.typeDir)+1))
}

// tasksPhpSkeleton renders a db/tasks.php stub for `component`.
func tasksPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$tasks = array(
    // TODO: register scheduled tasks here, e.g.:
    // array(
    //     'classname' => '%s\task\example_task',
    //     'blocking'  => 0,
    //     'minute'    => '*/30',
    //     'hour'      => '*',
    //     'day'       => '*',
    //     'dayofweek' => '*',
    //     'month'     => '*',
    // ),
);
`, component)
}

// servicesPhpSkeleton renders an empty db/services.php stub.
func servicesPhpSkeleton() string {
	return `<?php
defined('MOODLE_INTERNAL') || die();

$functions = array(
    // TODO: register web service functions here.
);
`
}

// eventsPhpSkeleton renders a db/events.php stub for `component`.
func eventsPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$observers = array(
    // TODO: register event observers here, e.g.:
    // array(
    //     'eventname' => '\core\event\user_loggedin',
    //     'callback'  => '\%s\observer::user_loggedin',
    // ),
);
`, component)
}

// observerPhpSkeleton renders the classes/observer.php stub for `component`.
func observerPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
namespace %s;

defined('MOODLE_INTERNAL') || die();

class observer {
    // TODO: implement the observer callbacks referenced in db/events.php.
}
`, component)
}

// accessPhpSkeleton renders a db/access.php stub for `component`.
func accessPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$capabilities = array(
    // TODO: define capabilities here, e.g.:
    // '%s:view' => array(
    //     'captype'      => 'read',
    //     'contextlevel' => CONTEXT_MODULE,
    //     'archetypes'   => array(
    //         'teacher' => CAP_ALLOW,
    //     ),
    // ),
);
`, component)
}

// settingsPhpSkeleton renders a settings.php stub for `component`.
func settingsPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

if ($hassiteconfig) {
    $settings = new admin_settingpage('%s', get_string('pluginname', '%s'));
    $ADMIN->add('localplugins', $settings); // TODO: adjust the admin category for this plugin type.

    // TODO: add settings here, e.g.:
    // $settings->add(new admin_setting_configtext('%s/example', get_string('example', '%s'), '', ''));
}
`, component, component, component, component)
}
