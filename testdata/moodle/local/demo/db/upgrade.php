<?php
defined('MOODLE_INTERNAL') || die();

function xmldb_local_demo_upgrade($oldversion) {
    global $DB;

    if ($oldversion < 2023120100) {
        // Add the initial records table
        $table = new xmldb_table('local_demo_records');
        upgrade_plugin_savepoint(true, 2023120100, 'local', 'demo');
    }

    if ($oldversion < 2024010100) { // Add the log table
        $table = new xmldb_table('local_demo_log');
        upgrade_plugin_savepoint(true, 2024010100, 'local', 'demo');
    }

    return true;
}
