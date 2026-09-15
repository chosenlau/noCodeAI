import React, { useState } from "react";

interface AddTodoProps {
    addTodo: (text: string) => void;
}

export default function AddTodo({ addTodo }: AddTodoProps) {
    const [text, setText] = useState("");
    const handleSubmit = (e: React.SubmitEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (text.trim() === '') {
            return
        }
        addTodo(text);
        setText('');
    }
    return (
        <form onSubmit={handleSubmit}>
            <input
                type="text"
                value={text}
                onChange={(e) => setText(e.target.value)}
            />
            <button type="submit" >Add</button>
        </form>
    );
}
