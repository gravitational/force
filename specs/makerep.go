// this is an imaginary replacement of Make target

all := Process(Spec{
	Watch: Files("*.go"),
	Run: Shell(`
       cd $HOME
       go build tele
	`),
})

// Make a makefile like program
// with help and stuff
Make()
