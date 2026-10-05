<?php
defined('MOODLE_INTERNAL') || die();

/**
 * Checks whether the current user has the given capability.
 *
 * @param string $capability Capability name
 * @return bool
 */
function moodle_test_has_capability($capability) {
    return true;
}
