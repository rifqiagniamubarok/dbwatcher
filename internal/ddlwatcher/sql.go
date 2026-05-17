package ddlwatcher

// notifyChannel is the LISTEN/NOTIFY channel used to carry DDL events from
// the Postgres event triggers back to DBWatch.
const notifyChannel = "dbwatch_ddl"

// Object names installed into the target database.
const (
	captureFuncName     = "dbwatch_ddl_capture"
	dropCaptureFuncName = "dbwatch_ddl_drop_capture"
	endTriggerName      = "dbwatch_ddl_watcher"
	dropTriggerName     = "dbwatch_ddl_drop_watcher"
)

// installSQL creates the capture functions and event triggers. It is
// idempotent: CREATE OR REPLACE for functions, and the trigger creates are
// guarded by InstallSQL's caller dropping them first (see Install).
//
// Two triggers are needed because ddl_command_end does not fire for DROP —
// sql_drop covers that case. pg_event_trigger_dropped_objects() is only
// valid inside an sql_drop trigger, hence the separate function.
const installSQL = `
CREATE OR REPLACE FUNCTION ` + captureFuncName + `()
RETURNS event_trigger AS $dbw$
DECLARE
  obj record;
BEGIN
  FOR obj IN SELECT * FROM pg_event_trigger_ddl_commands()
  LOOP
    PERFORM pg_notify('` + notifyChannel + `', json_build_object(
      'command_tag', obj.command_tag,
      'object_type', obj.object_type,
      'schema', obj.schema_name,
      'object_identity', obj.object_identity,
      'in_extension', obj.in_extension,
      'timestamp', extract(epoch from now())
    )::text);
  END LOOP;
END;
$dbw$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION ` + dropCaptureFuncName + `()
RETURNS event_trigger AS $dbw$
DECLARE
  obj record;
BEGIN
  FOR obj IN SELECT * FROM pg_event_trigger_dropped_objects()
  LOOP
    IF obj.original THEN
      PERFORM pg_notify('` + notifyChannel + `', json_build_object(
        'command_tag', 'DROP ' || upper(obj.object_type),
        'object_type', obj.object_type,
        'schema', obj.schema_name,
        'object_identity', obj.object_identity,
        'in_extension', obj.is_temporary,
        'timestamp', extract(epoch from now())
      )::text);
    END IF;
  END LOOP;
END;
$dbw$ LANGUAGE plpgsql;
`

// createTriggersSQL creates the two event triggers. Run after installSQL.
// Triggers are dropped first by Install so this stays effectively idempotent.
const createTriggersSQL = `
CREATE EVENT TRIGGER ` + endTriggerName + `
  ON ddl_command_end
  EXECUTE FUNCTION ` + captureFuncName + `();

CREATE EVENT TRIGGER ` + dropTriggerName + `
  ON sql_drop
  EXECUTE FUNCTION ` + dropCaptureFuncName + `();
`

// dropTriggersSQL removes the event triggers (IF EXISTS so it's safe to run
// when they were never installed).
const dropTriggersSQL = `
DROP EVENT TRIGGER IF EXISTS ` + endTriggerName + `;
DROP EVENT TRIGGER IF EXISTS ` + dropTriggerName + `;
`

// uninstallSQL removes triggers and functions entirely.
const uninstallSQL = dropTriggersSQL + `
DROP FUNCTION IF EXISTS ` + captureFuncName + `();
DROP FUNCTION IF EXISTS ` + dropCaptureFuncName + `();
`

// PrintSQL returns the full SQL a DBA can run by hand to install DDL
// tracking. Useful for the `dbwatch ddl-tools print-sql` subcommand.
func PrintSQL() string {
	return installSQL + dropTriggersSQL + createTriggersSQL
}
