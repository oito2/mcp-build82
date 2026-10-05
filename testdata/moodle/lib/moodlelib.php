<?php
defined('MOODLE_INTERNAL') || die();

/**
 * Returns the number of widgets configured for a course.
 *
 * @param int $courseid Course ID
 * @return int Number of widgets
 */
function moodle_test_public_function($courseid) {
    return 0;
}

/**
 * Old helper kept for backwards compatibility.
 *
 * @deprecated since Moodle 4.0, use moodle_test_public_function() instead
 */
function moodle_test_deprecated_function() {
    return moodle_test_public_function(0);
}

/**
 * Internal helper not meant for external use.
 *
 * @internal
 */
function moodle_test_internal_function() {
    return true;
}

/**
 * Private helper, naming-convention only.
 */
function _moodle_test_private_function() {
    return true;
}

function moodle_test_no_doc_function($x) {
    return $x;
}
