class ConnectionState {
	connected = $state(false);
	retrying = $state(false);
	set(c: boolean, r: boolean) {
		this.connected = c;
		this.retrying = r;
	}
}

export const connection = new ConnectionState();