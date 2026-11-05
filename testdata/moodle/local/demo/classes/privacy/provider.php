<?php
namespace local_demo\privacy;

defined('MOODLE_INTERNAL') || die();

/**
 * Privacy provider: local_demo stores no personal data.
 */
class provider implements \core_privacy\local\metadata\null_provider {
    public static function get_reason(): string {
        return 'privacy:metadata';
    }
}
