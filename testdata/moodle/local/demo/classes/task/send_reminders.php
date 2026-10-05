<?php
namespace local_demo\task;

class send_reminders extends \core\task\scheduled_task {
    public function get_name() {
        return 'Send reminders';
    }

    public function execute() {
        // Send reminder notifications.
    }
}
