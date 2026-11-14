UPDATE buddy.request_types SET fields = jsonb_set(fields, '{reason,required}', 'true') WHERE code='time_off';
