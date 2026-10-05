<?php
namespace local_demo\hook;

/**
 * Fired when data is submitted.
 */
class data_submitted {
    public function get_hook_description(): string {
        return 'Fired when a user submits data to the demo plugin.';
    }

    public function get_hook_tags(): array {
        return ['demo', 'data'];
    }
}
